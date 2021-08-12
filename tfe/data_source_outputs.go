package tfe

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"

	tfe "github.com/hashicorp/go-tfe"
	"github.com/hashicorp/terraform-plugin-go/tfprotov5"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/zclconf/go-cty/cty"
	ctyjson "github.com/zclconf/go-cty/cty/json"
)

type dataSourceWorkspaceOutputs struct {
	tfeClient *tfe.Client
}

var stderr *os.File

func init() {
	// There is a issue that occurs when the plugin-go Serve function is used that
	// causes os.Stderr to be overwritten. There is a fix being worked on for this.
	stderr = os.Stderr
}

func newDataSourceWorkspaceOutputs(client *tfe.Client) tfprotov5.DataSourceServer {
	return dataSourceWorkspaceOutputs{
		tfeClient: client,
	}
}

func (d dataSourceWorkspaceOutputs) ReadDataSource(ctx context.Context, req *tfprotov5.ReadDataSourceRequest) (*tfprotov5.ReadDataSourceResponse, error) {
	resp := &tfprotov5.ReadDataSourceResponse{
		Diagnostics: []*tfprotov5.Diagnostic{},
	}

	orgName, wsName, err := d.readConfigValues(req)
	if err != nil {
		resp.Diagnostics = append(resp.Diagnostics, &tfprotov5.Diagnostic{
			Severity: tfprotov5.DiagnosticSeverityError,
			Summary:  "Error retrieving values from the config",
			Detail:   fmt.Sprintf("Error retrieving values from the config: %v", err),
		})
		return resp, nil
	}

	remoteStateOutput, err := d.readStateOutput(ctx, d.tfeClient, orgName, wsName)
	if err != nil {
		resp.Diagnostics = append(resp.Diagnostics, &tfprotov5.Diagnostic{
			Severity: tfprotov5.DiagnosticSeverityError,
			Summary:  "Error reading remote state output",
			Detail:   fmt.Sprintf("Error reading remote state output: %v", err),
		})
		return resp, nil
	}

	tftypesValues, stateTypes, err := parseStateOutput(remoteStateOutput)
	if err != nil {
		resp.Diagnostics = append(resp.Diagnostics, &tfprotov5.Diagnostic{
			Severity: tfprotov5.DiagnosticSeverityError,
			Summary:  "Error parsing remote state output",
			Detail:   fmt.Sprintf("Error parsing remote state output: %v", err),
		})
		return resp, nil
	}

	id := fmt.Sprintf("%s-%s", orgName, wsName)
	state, err := tfprotov5.NewDynamicValue(tftypes.Object{
		AttributeTypes: map[string]tftypes.Type{
			"workspace":    tftypes.String,
			"organization": tftypes.String,
			"values":       tftypes.DynamicPseudoType,
			"id":           tftypes.String,
		},
	}, tftypes.NewValue(tftypes.Object{
		AttributeTypes: map[string]tftypes.Type{
			"workspace":    tftypes.String,
			"organization": tftypes.String,
			"values":       tftypes.Object{AttributeTypes: stateTypes},
			"id":           tftypes.String,
		},
	}, map[string]tftypes.Value{
		"workspace":    tftypes.NewValue(tftypes.String, wsName),
		"organization": tftypes.NewValue(tftypes.String, orgName),
		"values":       tftypes.NewValue(tftypes.Object{AttributeTypes: stateTypes}, tftypesValues),
		"id":           tftypes.NewValue(tftypes.String, id),
	}))

	if err != nil {
		return &tfprotov5.ReadDataSourceResponse{
			Diagnostics: []*tfprotov5.Diagnostic{
				{
					Severity: tfprotov5.DiagnosticSeverityError,
					Summary:  "Error encoding state",
					Detail:   fmt.Sprintf("Error encoding state: %s", err.Error()),
				},
			},
		}, nil
	}
	return &tfprotov5.ReadDataSourceResponse{
		State: &state,
	}, nil
}

func (d dataSourceWorkspaceOutputs) ValidateDataSourceConfig(ctx context.Context, req *tfprotov5.ValidateDataSourceConfigRequest) (*tfprotov5.ValidateDataSourceConfigResponse, error) {
	return &tfprotov5.ValidateDataSourceConfigResponse{}, nil
}

func (d dataSourceWorkspaceOutputs) readConfigValues(req *tfprotov5.ReadDataSourceRequest) (string, string, error) {
	var orgName string
	var wsName string
	var err error

	config := req.Config
	val, err := config.Unmarshal(tftypes.Object{
		AttributeTypes: map[string]tftypes.Type{
			"workspace":    tftypes.String,
			"organization": tftypes.String,
			"values":       tftypes.DynamicPseudoType,
			"id":           tftypes.String,
		}})
	if err != nil {
		return orgName, wsName, fmt.Errorf("Error unmarshalling config: %v", err)
	}

	var valMap map[string]tftypes.Value
	err = val.As(&valMap)
	if err != nil {
		return orgName, wsName, fmt.Errorf("Error assigning configuration attributes to map: %v", err)
	}

	if valMap["organization"].IsNull() || valMap["workspace"].IsNull() {
		return orgName, wsName, fmt.Errorf("Organization and Workspace cannot be nil: %v", err)
	}

	err = valMap["organization"].As(&orgName)
	if err != nil {
		return orgName, wsName, fmt.Errorf("Error assigning 'organization' value to string: %v", err)
	}

	err = valMap["workspace"].As(&wsName)
	if err != nil {
		return orgName, wsName, fmt.Errorf("Error assigning 'workspace' value to string: %v", err)
	}

	return orgName, wsName, nil
}

type rootModule struct {
	RootOutputs map[string]rawOutput `json:"outputs"`
}

type rawOutput struct {
	ValueRaw     json.RawMessage `json:"value"`
	ValueTypeRaw json.RawMessage `json:"type"`
}

type outputData struct {
	Value cty.Value
}

type stateData struct {
	outputs map[string]*outputData
}

func (d dataSourceWorkspaceOutputs) readStateOutput(ctx context.Context, tfeClient *tfe.Client, orgName, wsName string) (*stateData, error) {
	log.Printf("[DEBUG] Reading the Workspace %s in Organization %s", wsName, orgName)
	ws, err := tfeClient.Workspaces.Read(ctx, orgName, wsName)
	if err != nil {
		return nil, fmt.Errorf("Error reading workspace: %v", err)
	}

	log.Printf("[DEBUG] Reading the current StateVersion for Workspace ID %s.", ws.ID)
	sv, err := tfeClient.StateVersions.Current(ctx, ws.ID)
	if err != nil {
		if err == tfe.ErrResourceNotFound {
			return nil, fmt.Errorf("Current remote state for workspace '%s' not found.", wsName)
		}
		return nil, fmt.Errorf("Could not read the current state for workspace '%s' : %v", wsName, err)
	}

	log.Printf("[DEBUG] Downloading State Version")
	rawState, err := tfeClient.StateVersions.Download(ctx, sv.DownloadURL)
	if err != nil {
		return nil, fmt.Errorf("Error downloading state: %v", err)
	}

	log.Printf("[DEBUG] Unmarshalling state output")
	read := bytes.NewReader(rawState)
	src, err := ioutil.ReadAll(read)
	if err != nil {
		return nil, fmt.Errorf("Could not read state data: %v", err)
	}
	rrs := &rootModule{}
	err = json.Unmarshal(src, rrs)
	if err != nil {
		return nil, fmt.Errorf("Could not unmarshal state data: %v", err)
	}

	fov := &stateData{
		outputs: map[string]*outputData{},
	}
	for name, fos := range rrs.RootOutputs {
		ty, err := ctyjson.UnmarshalType([]byte(fos.ValueTypeRaw))
		if err != nil {
			return nil, fmt.Errorf("Could not unmarshal type: %v", err)
		}

		val, err := ctyjson.Unmarshal([]byte(fos.ValueRaw), ty)
		if err != nil {
			return nil, fmt.Errorf("Could not unmarshal value: %v", err)
		}

		fov.outputs[name] = &outputData{
			Value: val,
		}
	}

	return fov, nil
}

func parseStateOutput(stateOutput *stateData) (map[string]tftypes.Value, map[string]tftypes.Type, error) {
	tftypesValues := map[string]tftypes.Value{}
	stateTypes := map[string]tftypes.Type{}

	for name, output := range stateOutput.outputs {
		marshData, err := output.Value.Type().MarshalJSON()
		if err != nil {
			return nil, nil, fmt.Errorf("Could not marshal output type: %v", err)
		}
		tfType, err := tftypes.ParseJSONType(marshData)
		if err != nil {
			return nil, nil, fmt.Errorf("Could not parse json type data: %v", err)
		}
		mByte, err := ctyjson.Marshal(output.Value, output.Value.Type())
		if err != nil {
			return nil, nil, fmt.Errorf("Could not marshal output value and output type: %v", err)
		}
		tfRawState := tfprotov5.RawState{
			JSON: mByte,
		}
		newVal, err := tfRawState.Unmarshal(tfType)
		if err != nil {
			return nil, nil, fmt.Errorf("Could not unmarshal tftype into value: %v", err)
		}
		tftypesValues[name] = newVal
		stateTypes[name] = tfType
	}

	return tftypesValues, stateTypes, nil
}
