package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &GCPRoleResource{}

// GCPRoleResource manages a Tuck GCP dynamic-secrets role.
type GCPRoleResource struct {
	client *tuckClient
}

func NewGCPRoleResource() resource.Resource {
	return &GCPRoleResource{}
}

func (r *GCPRoleResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_gcp_role"
}

func (r *GCPRoleResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Tuck GCP dynamic-secrets role — generate_creds against this role returns a short-lived service account key or OAuth2 access token. The role name is immutable; changing it forces replacement.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Required:    true,
				Description: "Unique GCP role name.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"credential_type": schema.StringAttribute{
				Required:    true,
				Description: "\"service_account_key\" (a new key is minted per credential) or \"access_token\" (a short-lived OAuth2 token via impersonation).",
			},
			"service_account_email": schema.StringAttribute{
				Required:    true,
				Description: "GCP service account this role generates credentials for.",
			},
			"key_algorithm": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "\"KEY_ALG_RSA_2048\" (default) or \"KEY_ALG_RSA_4096\". Only meaningful for credential_type = \"service_account_key\".",
			},
			"scopes": schema.ListAttribute{
				Optional:    true,
				Computed:    true,
				ElementType: types.StringType,
				Description: "OAuth2 scopes requested. Only meaningful for credential_type = \"access_token\".",
			},
			"default_ttl": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Default credential lifetime (e.g. \"1h\") when generate_creds doesn't specify one.",
			},
			"max_ttl": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Maximum credential lifetime.",
			},
		},
	}
}

type gcpRoleModel struct {
	Name                types.String `tfsdk:"name"`
	CredentialType      types.String `tfsdk:"credential_type"`
	ServiceAccountEmail types.String `tfsdk:"service_account_email"`
	KeyAlgorithm        types.String `tfsdk:"key_algorithm"`
	Scopes              types.List   `tfsdk:"scopes"`
	DefaultTTL          types.String `tfsdk:"default_ttl"`
	MaxTTL              types.String `tfsdk:"max_ttl"`
}

func (r *GCPRoleResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *GCPRoleResource) writeAndRead(ctx context.Context, name string, apiReq gcpRoleReq) (*gcpRoleAPIResp, error) {
	if err := r.client.putGCPRole(ctx, name, apiReq); err != nil {
		return nil, err
	}
	role, found, err := r.client.getGCPRole(ctx, name)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, errRoleDisappearedAfterWrite(name)
	}
	return role, nil
}

func (r *GCPRoleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan gcpRoleModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	result, err := r.writeAndRead(ctx, plan.Name.ValueString(), gcpRoleModelToReq(ctx, plan))
	if err != nil {
		resp.Diagnostics.AddError("Error creating GCP role", err.Error())
		return
	}
	state, diags := gcpRoleRespToModel(ctx, result)
	preservePlannedTTLs(&state.DefaultTTL, &state.MaxTTL, plan.DefaultTTL, plan.MaxTTL)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *GCPRoleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state gcpRoleModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	role, found, err := r.client.getGCPRole(ctx, state.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading GCP role", err.Error())
		return
	}
	if !found {
		resp.State.RemoveResource(ctx)
		return
	}
	newState, diags := gcpRoleRespToModel(ctx, role)
	newState.DefaultTTL = types.StringValue(preserveEquivalentTTL(newState.DefaultTTL.ValueString(), state.DefaultTTL))
	newState.MaxTTL = types.StringValue(preserveEquivalentTTL(newState.MaxTTL.ValueString(), state.MaxTTL))
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, newState)...)
}

func (r *GCPRoleResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan gcpRoleModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	result, err := r.writeAndRead(ctx, plan.Name.ValueString(), gcpRoleModelToReq(ctx, plan))
	if err != nil {
		resp.Diagnostics.AddError("Error updating GCP role", err.Error())
		return
	}
	state, diags := gcpRoleRespToModel(ctx, result)
	preservePlannedTTLs(&state.DefaultTTL, &state.MaxTTL, plan.DefaultTTL, plan.MaxTTL)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *GCPRoleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state gcpRoleModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.deleteGCPRole(ctx, state.Name.ValueString()); err != nil {
		resp.Diagnostics.AddError("Error deleting GCP role", err.Error())
	}
}

func gcpRoleModelToReq(ctx context.Context, m gcpRoleModel) gcpRoleReq {
	var scopes []string
	_ = m.Scopes.ElementsAs(ctx, &scopes, false)
	return gcpRoleReq{
		CredentialType:      m.CredentialType.ValueString(),
		ServiceAccountEmail: m.ServiceAccountEmail.ValueString(),
		KeyAlgorithm:        m.KeyAlgorithm.ValueString(),
		Scopes:              scopes,
		DefaultTTL:          m.DefaultTTL.ValueString(),
		MaxTTL:              m.MaxTTL.ValueString(),
	}
}

func gcpRoleRespToModel(ctx context.Context, role *gcpRoleAPIResp) (gcpRoleModel, diag.Diagnostics) {
	scopes, diags := types.ListValueFrom(ctx, types.StringType, role.Scopes)
	return gcpRoleModel{
		Name:                types.StringValue(role.Name),
		CredentialType:      types.StringValue(role.CredentialType),
		ServiceAccountEmail: types.StringValue(role.ServiceAccountEmail),
		KeyAlgorithm:        types.StringValue(role.KeyAlgorithm),
		Scopes:              scopes,
		DefaultTTL:          types.StringValue(nsDuration(role.DefaultTTL)),
		MaxTTL:              types.StringValue(nsDuration(role.MaxTTL)),
	}, diags
}
