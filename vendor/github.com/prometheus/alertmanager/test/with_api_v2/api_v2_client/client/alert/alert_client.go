// Code generated by go-swagger; DO NOT EDIT.

package alert

// This file was generated by the swagger tool.
// Editing this file might prove futile when you re-run the swagger generate command

import (
	"github.com/go-openapi/runtime"

	strfmt "github.com/go-openapi/strfmt"
)

// New creates a new alert API client.
func New(transport runtime.ClientTransport, formats strfmt.Registry) *Client {
	return &Client{transport: transport, formats: formats}
}

/*
Client for alert API
*/
type Client struct {
	transport runtime.ClientTransport
	formats   strfmt.Registry
}

/*
GetAlerts Get a list of alerts
*/
func (a *Client) GetAlerts(params *GetAlertsParams) (*GetAlertsOK, error) {
	// TODO: Validate the params before sending
	if params == nil {
		params = NewGetAlertsParams()
	}

	result, err := a.transport.Submit(&runtime.ClientOperation{
		ID:                 "getAlerts",
		Method:             "GET",
		PathPattern:        "/alerts",
		ProducesMediaTypes: []string{"application/json"},
		ConsumesMediaTypes: []string{"application/json"},
		Schemes:            []string{"http"},
		Params:             params,
		Reader:             &GetAlertsReader{formats: a.formats},
		Context:            params.Context,
		Client:             params.HTTPClient,
	})
	if err != nil {
		return nil, err
	}
	return result.(*GetAlertsOK), nil

}

/*
PostAlerts Create new Alerts
*/
func (a *Client) PostAlerts(params *PostAlertsParams) (*PostAlertsOK, error) {
	// TODO: Validate the params before sending
	if params == nil {
		params = NewPostAlertsParams()
	}

	result, err := a.transport.Submit(&runtime.ClientOperation{
		ID:                 "postAlerts",
		Method:             "POST",
		PathPattern:        "/alerts",
		ProducesMediaTypes: []string{"application/json"},
		ConsumesMediaTypes: []string{"application/json"},
		Schemes:            []string{"http"},
		Params:             params,
		Reader:             &PostAlertsReader{formats: a.formats},
		Context:            params.Context,
		Client:             params.HTTPClient,
	})
	if err != nil {
		return nil, err
	}
	return result.(*PostAlertsOK), nil

}

// SetTransport changes the transport on the client
func (a *Client) SetTransport(transport runtime.ClientTransport) {
	a.transport = transport
}
