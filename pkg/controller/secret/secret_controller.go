package secret

import (
	"context"
	"fmt"

	"regexp"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"

	alertmanager "github.com/openshift/configure-alertmanager-operator/pkg/types"

	yaml "gopkg.in/yaml.v2"
)

var log = logf.Log.WithName("controller_secret")

// Add creates a new Secret Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileSecret{client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("secret-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource (type "Secret").
	// For each Add/Update/Delete event, the reconcile loop will be sent a reconcile Request.
	err = c.Watch(&source.Kind{Type: &corev1.Secret{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}
	return nil
}

var _ reconcile.Reconciler = &ReconcileSecret{}

// ReconcileSecret reconciles a Secret object
type ReconcileSecret struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client client.Client
	scheme *runtime.Scheme
}

// Reconcile reads that state of the cluster for a Secret object and makes changes based on the state read.
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileSecret) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling Secret")

	// This operator is only interested in the 3 secrets listed below. Skip reconciling for all other secrets.
	if request.Name == "alertmanager-main" || request.Name == "dms-secret" || request.Name == "pd-secret" {
		instance := &corev1.Secret{}
		err := r.client.Get(context.TODO(), request.NamespacedName, instance)
		if err != nil {
			if errors.IsNotFound(err) {
				// Request object not found, could have been deleted after reconcile request.
				// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
				// Return and don't requeue
				return reconcile.Result{}, nil
			}
			// Error reading the object - requeue the request.
			return reconcile.Result{}, err
		}

		// Configure the query to search the 'openshift-monitoring' namespace for Secrets.
		secretList := &corev1.SecretList{}
		opts := client.ListOptions{Namespace: request.Namespace}
		if err := opts.SetLabelSelector("k8s-app=alertmanager-config-operator"); err != nil {
			reqLogger.Error(err, "Failed to SetLabelSelector for SecretList")
		}

		reqLogger.Info("Starting reconcile for secret: ", instance.Name)

		// List all the secrets matching the above Namespace and LabelSelector.
		r.client.List(context.TODO(), &opts, secretList)

		// Check for the presence of specific secrets.
		pagerDutySecretExists := secretInList("pd-secret", secretList)
		snitchSecretExists := secretInList("dms-secret", secretList)

		// Extract the alertmanager config from the alertmanager-main secret.
		alertmanagerconfig := getAlertManagerConfig(r, &request)
		fmt.Println(alertmanagerconfig)

		// Determine if any updates need to happen to the Alertmanager config and make the changes in memory.
		amconfigneedsupdate := false
		if pagerDutySecretExists {
			log.Info("Pager Duty secret exists")
			pdsecret := getSecretKey(r, &request, "pd-secret", "PAGERDUTY_KEY")
			addPDSecretToAlertManagerConfig(r, &request, &alertmanagerconfig, pdsecret)
			amconfigneedsupdate = true
		} else {
			log.Info("Pager Duty secret is absent")
			removeConfigFromAlertManager(r, &request, &alertmanagerconfig, "pager duty")
			amconfigneedsupdate = true
		}
		if snitchSecretExists {
			log.Info("Dead Man's Snitch secret exists")
			snitchsecret := getSecretKey(r, &request, "dms-secret", "SNITCH_URL")
			addSnitchSecretToAlertManagerConfig(r, &request, &alertmanagerconfig, snitchsecret)
			amconfigneedsupdate = true
		} else {
			log.Info("Dead Man's Snitch secret is absent")
			removeConfigFromAlertManager(r, &request, &alertmanagerconfig, "watchdog")
			amconfigneedsupdate = true
		}

		// Commit any changes to the Alertmanager config.
		// This takes the changes that are in memory and writes to the kubernetes cluster.
		if amconfigneedsupdate {
			log.Info("Writing changes to Alertmanager config.")
			updateAlertManagerConfig(r, &request, &alertmanagerconfig)
		}

	} else {
		reqLogger.Info("Skip reconcile: No changes detected to alertmanager secrets.")
		return reconcile.Result{}, nil
	}
	reqLogger.Info("Finished reconcile for secret.")
	return reconcile.Result{}, nil
}

// secretInList takes the name of Secret, and a list of Secrets, and returns a Bool
// indicating if the name is present in the list
func secretInList(name string, list *corev1.SecretList) bool {
	for _, secret := range list.Items {
		if name == secret.Name {
			log.Info("Secret named", secret.Name, "found")
			return true
		}
	}
	log.Info("Secret", name, "not found")
	return false
}

// getSecretKey fetches the data from a Secret, such as a PagerDuty API key.
func getSecretKey(r *ReconcileSecret, request *reconcile.Request, secretname string, fieldname string) string {

	secret := &corev1.Secret{}

	// Define a new objectKey for fetching the secret key.
	objectKey := client.ObjectKey{
		Namespace: request.Namespace,
		Name:      secretname,
	}

	// Fetch the key from the secret object.
	r.client.Get(context.TODO(), objectKey, secret)
	secretkey := secret.Data[fieldname]

	return string(secretkey)
}

// getAlertManagerConfig fetches the AlertManager configuration from its default location.
// This is equivalent to `oc get secrets -n openshift-monitoring alertmanager-main`.
// It specifically extracts the .data "alertmanager.yaml" field, and loads it into a resource
// of type Config, as defined by the Alertmanager package.
func getAlertManagerConfig(r *ReconcileSecret, request *reconcile.Request) alertmanager.Config {

	amconfig := alertmanager.Config{}

	secret := &corev1.Secret{}

	// Define a new objectKey for fetching the alertmanager config.
	objectKey := client.ObjectKey{
		Namespace: request.Namespace,
		Name:      "alertmanager-main",
	}

	// Fetch the alertmanager config and load it into an alertmanager.Config struct.
	r.client.Get(context.TODO(), objectKey, secret)
	secretdata := secret.Data["alertmanager.yaml"]
	err := yaml.Unmarshal(secretdata, &amconfig)
	if err != nil {
		panic(err)
	}

	return amconfig
}

// addPDSecretToAlertManagerConfig adds the Pager Duty integration settings into the existing Alertmanager config.
// The changes are kept in memory until committed using function updateAlertManagerConfig().
func addPDSecretToAlertManagerConfig(r *ReconcileSecret, request *reconcile.Request, amconfig *alertmanager.Config, pdsecret string) {

	// Define the contents of the PagerDutyConfig.
	pdconfig := &alertmanager.PagerdutyConfig{
		NotifierConfig: alertmanager.NotifierConfig{VSendResolved: true},
		RoutingKey:     pdsecret,
		Description:    `{{ .CommonLabels.alertname }} {{ .CommonLabels.severity | toUpper }} ({{ len .Alerts }})`,
		Details: map[string]string{
			"link":         `{{ .CommonAnnotations.link }}?`,
			"group":        `{{ .CommonLabels.alertname }}`,
			"component":    `{{ .CommonLabels.alertname }}`,
			"num_firing":   `{{ .Alerts.Firing | len }}`,
			"num_resolved": `{{ .Alerts.Resolved | len }}`,
			"resolved":     `{{ template pagerduty.default.instances .Alerts.Resolved }}`,
		},
	}

	// Overwrite the existing Pager Duty config with the updated version specified above.
	// This keeps other receivers intact while updating only the Pager Duty receiver.
	pagerdutyabsent := true
	for i, receiver := range amconfig.Receivers {
		fmt.Println("Found Receiver named:", receiver.Name)
		if receiver.Name == "pagerduty" {
			fmt.Println("Overwriting Pager Duty config for Receiver:", receiver.Name)
			amconfig.Receivers[i].PagerdutyConfigs = []*alertmanager.PagerdutyConfig{pdconfig}
			pagerdutyabsent = false
		} else {
			fmt.Println("SKipping Receiver named", receiver.Name)
		}
	}

	// Create the Pager Duty config if it doesn't already exist.
	if pagerdutyabsent {
		fmt.Println("Pager Duty receiver is absent. Creating new receiver.")
		newreceiver := &alertmanager.Receiver{
			Name:             "pagerduty",
			PagerdutyConfigs: []*alertmanager.PagerdutyConfig{pdconfig},
		}
		amconfig.Receivers = append(amconfig.Receivers, newreceiver)
	}

	// Create a route for the new Pager Duty receiver
	pdroute := &alertmanager.Route{
		Continue: true,
		Receiver: "pagerduty",
		GroupByStr: []string{
			"alertname",
			"severity",
		},
		MatchRE: map[string]alertmanager.Regexp{
			"namespace": {
				Regexp: regexp.MustCompile("^openshift$|openshift-.*|default$|kube$|kube-.*|logging$"),
			},
		},
	}

	// Insert the Route for the Pager Duty Receiver.
	routeabsent := true
	for i, route := range amconfig.Route.Routes {
		fmt.Println("Found Route for Receiver:", route.Receiver)
		if route.Receiver == "pagerduty" {
			fmt.Println("Overwriting Pager Duty Route for Receiver:", route.Receiver)
			amconfig.Route.Routes[i] = pdroute
			routeabsent = false
		} else {
			fmt.Println("SKipping Route for Receiver named", route.Receiver)
		}
	}

	// Create Route for Pager Duty Receiver if it doesn't already exist.
	if routeabsent {
		fmt.Println("Route for Pager Duty Receiver is absent. Creating new Route.")
		amconfig.Route.Routes = append(amconfig.Route.Routes, pdroute)
	}
}

// updateAlertManagerConfig writes the updated alertmanager config to the `alertmanager-main` secret in namespace `openshift-monitoring`.
func updateAlertManagerConfig(r *ReconcileSecret, request *reconcile.Request, amconfig *alertmanager.Config) {

	amconfigbyte, marshalerr := yaml.Marshal(amconfig)
	if marshalerr != nil {
		log.Error(marshalerr, "Error marshalling Alertmanager config")
	}
	fmt.Println("Debug Marshalled Alertmanager config:", string(amconfigbyte))

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "alertmanager-main",
			Namespace: "openshift-monitoring",
		},
		Data: map[string][]byte{
			"alertmanager.yaml": amconfigbyte,
		},
	}

	// Write the alertmanager config into the alertmanager secret.
	err := r.client.Update(context.TODO(), secret)
	if err != nil {
		log.Error(err, "Could not write secret alertmanger-main in namespace", request.Namespace)
		return
	}
	log.Info("Secret alertmanager-main successfully updated")
}

// addSnitchSecretToAlertManagerConfig adds the Dead Man's Snitch settings into the existing Alertmanager config.
// The changes are kept in memory until committed using function updateAlertManagerConfig().
func addSnitchSecretToAlertManagerConfig(r *ReconcileSecret, request *reconcile.Request, amconfig *alertmanager.Config, snitchsecret string) {

	// Define the contents of the WebhookConfig which is part of the Watchdog receiver.
	// The Watchdog receiver uses the Dead Man's Snitch external service as its webhook.
	snitchconfig := &alertmanager.WebhookConfig{URL: snitchsecret}

	// Overwrite the existing Watchdog config with the updated version specified above.
	// This keeps other receivers intact while updating only the Watchdog receiver.
	watchdogabsent := true
	for i, receiver := range amconfig.Receivers {
		fmt.Println("Found Receiver named:", receiver.Name)
		if receiver.Name == "watchdog" {
			fmt.Println("Overwriting watchdog receiver:", receiver.Name)
			amconfig.Receivers[i].WebhookConfigs = []*alertmanager.WebhookConfig{snitchconfig}
			watchdogabsent = false
		} else {
			fmt.Println("SKipping Receiver named", receiver.Name)
		}
	}

	// Create the Watchdog receiver if it doesn't already exist.
	if watchdogabsent {
		fmt.Println("Watchdog receiver is absent. Creating new receiver.")
		newreceiver := &alertmanager.Receiver{
			Name:           "watchdog",
			WebhookConfigs: []*alertmanager.WebhookConfig{snitchconfig},
		}
		amconfig.Receivers = append(amconfig.Receivers, newreceiver)
	}

	// Create a route for the new Watchdog receiver.
	wdroute := &alertmanager.Route{
		Receiver:       "watchdog",
		RepeatInterval: "5m",
		Match:          map[string]string{"alertname": "Watchdog"},
	}

	// Insert the Route for the Watchdog Receiver.
	routeabsent := true
	for i, route := range amconfig.Route.Routes {
		fmt.Println("Found Route for Receiver:", route.Receiver)
		if route.Receiver == "watchdog" {
			fmt.Println("Overwriting Watchdog Route for Receiver:", route.Receiver)
			amconfig.Route.Routes[i] = wdroute
			routeabsent = false
		} else {
			fmt.Println("SKipping Route for Receiver named", route.Receiver)
		}
	}

	// Create Route for Watchdog Receiver if it doesn't already exist.
	if routeabsent {
		fmt.Println("Route for Watchdog Receiver is absent. Creating new Route.")
		amconfig.Route.Routes = append(amconfig.Route.Routes, wdroute)
	}
}

// removeFromReceivers removes the specified index from a slice of Receivers.
func removeFromReceivers(slice []*alertmanager.Receiver, i int) []*alertmanager.Receiver {
	copy(slice[i:], slice[i+1:])
	return slice[:len(slice)-1]
}

// removeFromRoutes removes the specified index from a slice of Routes.
func removeFromRoutes(slice []*alertmanager.Route, i int) []*alertmanager.Route {
	copy(slice[i:], slice[i+1:])
	return slice[:len(slice)-1]
}

// removeConfigFromAlertManager removes a Receiver config and the associated Route from Alertmanager.
// The changes are kept in memory until committed using function updateAlertManagerConfig().
func removeConfigFromAlertManager(r *ReconcileSecret, request *reconcile.Request, amconfig *alertmanager.Config, receivername string) {
	for i, receiver := range amconfig.Receivers {
		fmt.Println("Found Receiver named:", receiver.Name)
		if receiver.Name == receivername {
			fmt.Println("Deleting receiver named:", receiver.Name)
			removeFromReceivers(amconfig.Receivers, i)
		} else {
			fmt.Println("SKipping Receiver named", receiver.Name)
		}
	}
	for i, route := range amconfig.Route.Routes {
		fmt.Println("Found Route for Receiver:", route.Receiver)
		if route.Receiver == receivername {
			fmt.Println("Deleting Route for Receiver:", route.Receiver)
			removeFromRoutes(amconfig.Route.Routes, i)
		} else {
			fmt.Println("SKipping Route for Receiver named", route.Receiver)
		}
	}
}
