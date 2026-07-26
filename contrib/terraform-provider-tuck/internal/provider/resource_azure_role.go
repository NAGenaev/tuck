package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &AzureRoleResource{}

// AzureRoleResource manages a Tuck Azure dynamic-secrets role.
type AzureRoleResource struct {
	client *tuckClient
}

func NewAzureRoleResource() resource.Resource {
	return &AzureRoleResource{}
}

func (r *AzureRoleResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_azure_role"
}

func (r *AzureRoleResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Tuck Azure dynamic-secrets role — generate_creds against this role returns a short-lived Azure AD application credential. The role name is immutable; changing it forces replacement.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Required:    true,
				Description: "Unique Azure role name.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"application_object_id": schema.StringAttribute{
				Required:    true,
				Description: "Azure AD application object ID used for Graph API calls when generating credentials.",
			},
			"application_id": schema.StringAttribute{
				Required:    true,
				Description: "Azure AD application (client) ID returned to callers alongside generated credentials.",
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

type azureRoleModel struct {
	Name                types.String `tfsdk:"name"`
	ApplicationObjectID types.String `tfsdk:"application_object_id"`
	ApplicationID       types.String `tfsdk:"application_id"`
	DefaultTTL          types.String `tfsdk:"default_ttl"`
	MaxTTL              types.String `tfsdk:"max_ttl"`
}

func (r *AzureRoleResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *AzureRoleResource) writeAndRead(ctx context.Context, name string, apiReq azureRoleReq) (*azureRoleAPIResp, error) {
	if err := r.client.putAzureRole(ctx, name, apiReq); err != nil {
		return nil, err
	}
	role, found, err := r.client.getAzureRole(ctx, name)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, errRoleDisappearedAfterWrite(name)
	}
	return role, nil
}

func (r *AzureRoleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan azureRoleModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	result, err := r.writeAndRead(ctx, plan.Name.ValueString(), azureRoleModelToReq(plan))
	if err != nil {
		resp.Diagnostics.AddError("Error creating Azure role", err.Error())
		return
	}
	state := azureRoleRespToModel(result)
	preservePlannedTTLs(&state.DefaultTTL, &state.MaxTTL, plan.DefaultTTL, plan.MaxTTL)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *AzureRoleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state azureRoleModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	role, found, err := r.client.getAzureRole(ctx, state.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading Azure role", err.Error())
		return
	}
	if !found {
		resp.State.RemoveResource(ctx)
		return
	}
	newState := azureRoleRespToModel(role)
	newState.DefaultTTL = types.StringValue(preserveEquivalentTTL(newState.DefaultTTL.ValueString(), state.DefaultTTL))
	newState.MaxTTL = types.StringValue(preserveEquivalentTTL(newState.MaxTTL.ValueString(), state.MaxTTL))
	resp.Diagnostics.Append(resp.State.Set(ctx, newState)...)
}

func (r *AzureRoleResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan azureRoleModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	result, err := r.writeAndRead(ctx, plan.Name.ValueString(), azureRoleModelToReq(plan))
	if err != nil {
		resp.Diagnostics.AddError("Error updating Azure role", err.Error())
		return
	}
	state := azureRoleRespToModel(result)
	preservePlannedTTLs(&state.DefaultTTL, &state.MaxTTL, plan.DefaultTTL, plan.MaxTTL)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *AzureRoleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state azureRoleModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.deleteAzureRole(ctx, state.Name.ValueString()); err != nil {
		resp.Diagnostics.AddError("Error deleting Azure role", err.Error())
	}
}

func azureRoleModelToReq(m azureRoleModel) azureRoleReq {
	return azureRoleReq{
		ApplicationObjectID: m.ApplicationObjectID.ValueString(),
		ApplicationID:       m.ApplicationID.ValueString(),
		DefaultTTL:          m.DefaultTTL.ValueString(),
		MaxTTL:              m.MaxTTL.ValueString(),
	}
}

func azureRoleRespToModel(role *azureRoleAPIResp) azureRoleModel {
	return azureRoleModel{
		Name:                types.StringValue(role.Name),
		ApplicationObjectID: types.StringValue(role.ApplicationObjectID),
		ApplicationID:       types.StringValue(role.ApplicationID),
		DefaultTTL:          types.StringValue(nsDuration(role.DefaultTTL)),
		MaxTTL:              types.StringValue(nsDuration(role.MaxTTL)),
	}
}
