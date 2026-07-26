package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &GCPConfigResource{}

// GCPConfigResource manages Tuck's GCP dynamic-secrets engine config. This
// is a singleton on the server (PUT/GET/DELETE /v1/gcp/config, no name in
// the path) — the Terraform resource address itself gives it identity.
type GCPConfigResource struct {
	client *tuckClient
}

func NewGCPConfigResource() resource.Resource {
	return &GCPConfigResource{}
}

func (r *GCPConfigResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_gcp_config"
}

func (r *GCPConfigResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages Tuck's GCP dynamic-secrets engine configuration (singleton — one per Tuck server/namespace).",
		Attributes: map[string]schema.Attribute{
			"credentials_json": schema.StringAttribute{
				Required:    true,
				Sensitive:   true,
				Description: "GCP service account key JSON used to manage IAM service accounts/keys. Tuck never returns this value back on read (the API redacts it), so Terraform always trusts the configured value rather than server state for this attribute.",
			},
		},
	}
}

type gcpConfigModel struct {
	CredentialsJSON types.String `tfsdk:"credentials_json"`
}

func (r *GCPConfigResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *GCPConfigResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan gcpConfigModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.putGCPConfig(ctx, gcpConfigReq{CredentialsJSON: plan.CredentialsJSON.ValueString()}); err != nil {
		resp.Diagnostics.AddError("Error creating GCP config", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *GCPConfigResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state gcpConfigModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	_, found, err := r.client.getGCPConfig(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Error reading GCP config", err.Error())
		return
	}
	if !found {
		resp.State.RemoveResource(ctx)
		return
	}
	// credentials_json comes back blanked from the API — the whole resource
	// is that one sensitive field, so Read just confirms the config still
	// exists and otherwise leaves state (Terraform's own record of what it
	// configured) untouched.
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *GCPConfigResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan gcpConfigModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.putGCPConfig(ctx, gcpConfigReq{CredentialsJSON: plan.CredentialsJSON.ValueString()}); err != nil {
		resp.Diagnostics.AddError("Error updating GCP config", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *GCPConfigResource) Delete(ctx context.Context, _ resource.DeleteRequest, resp *resource.DeleteResponse) {
	if err := r.client.deleteGCPConfig(ctx); err != nil {
		resp.Diagnostics.AddError("Error deleting GCP config", err.Error())
	}
}
