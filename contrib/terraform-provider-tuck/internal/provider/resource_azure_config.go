package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &AzureConfigResource{}

// AzureConfigResource manages Tuck's Azure dynamic-secrets engine config.
// This is a singleton on the server (PUT/GET/DELETE /v1/azure/config, no
// name in the path) — the Terraform resource address itself gives it
// identity.
type AzureConfigResource struct {
	client *tuckClient
}

func NewAzureConfigResource() resource.Resource {
	return &AzureConfigResource{}
}

func (r *AzureConfigResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_azure_config"
}

func (r *AzureConfigResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages Tuck's Azure dynamic-secrets engine configuration (singleton — one per Tuck server/namespace).",
		Attributes: map[string]schema.Attribute{
			"tenant_id": schema.StringAttribute{
				Required:    true,
				Description: "Azure AD tenant ID.",
			},
			"client_id": schema.StringAttribute{
				Required:    true,
				Description: "Azure AD application (client) ID used to manage app registrations/credentials.",
			},
			"client_secret": schema.StringAttribute{
				Required:    true,
				Sensitive:   true,
				Description: "Azure AD application client secret. Tuck never returns this value back on read (the API redacts it), so Terraform always trusts the configured value rather than server state for this attribute.",
			},
		},
	}
}

type azureConfigModel struct {
	TenantID     types.String `tfsdk:"tenant_id"`
	ClientID     types.String `tfsdk:"client_id"`
	ClientSecret types.String `tfsdk:"client_secret"`
}

func (r *AzureConfigResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	client, ok := req.ProviderData.(*tuckClient)
	if !ok {
		resp.Diagnostics.AddError("unexpected provider data type",
			"Expected *tuckClient from provider.Configure.")
		return
	}
	r.client = client
}

func (r *AzureConfigResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan azureConfigModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.putAzureConfig(ctx, azureConfigModelToReq(plan)); err != nil {
		resp.Diagnostics.AddError("Error creating Azure config", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *AzureConfigResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state azureConfigModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	cfg, found, err := r.client.getAzureConfig(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Error reading Azure config", err.Error())
		return
	}
	if !found {
		resp.State.RemoveResource(ctx)
		return
	}
	newState := azureConfigModel{
		TenantID: types.StringValue(cfg.TenantID),
		ClientID: types.StringValue(cfg.ClientID),
		// client_secret comes back blanked from the API — keep whatever
		// Terraform already has, never overwrite it from a read.
		ClientSecret: state.ClientSecret,
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, newState)...)
}

func (r *AzureConfigResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan azureConfigModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.putAzureConfig(ctx, azureConfigModelToReq(plan)); err != nil {
		resp.Diagnostics.AddError("Error updating Azure config", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *AzureConfigResource) Delete(ctx context.Context, _ resource.DeleteRequest, resp *resource.DeleteResponse) {
	if err := r.client.deleteAzureConfig(ctx); err != nil {
		resp.Diagnostics.AddError("Error deleting Azure config", err.Error())
	}
}

func azureConfigModelToReq(m azureConfigModel) azureConfigReq {
	return azureConfigReq{
		TenantID:     m.TenantID.ValueString(),
		ClientID:     m.ClientID.ValueString(),
		ClientSecret: m.ClientSecret.ValueString(),
	}
}
