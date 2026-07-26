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

var _ resource.Resource = &AWSRoleResource{}

// AWSRoleResource manages a Tuck AWS dynamic-secrets role.
type AWSRoleResource struct {
	client *tuckClient
}

func NewAWSRoleResource() resource.Resource {
	return &AWSRoleResource{}
}

func (r *AWSRoleResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_aws_role"
}

func (r *AWSRoleResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Tuck AWS dynamic-secrets role — generate_creds against this role returns short-lived IAM credentials. The role name is immutable; changing it forces replacement.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Required:    true,
				Description: "Unique AWS role name.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"credential_type": schema.StringAttribute{
				Required:    true,
				Description: "\"iam_user\" (a dedicated IAM user is created per credential) or \"assumed_role\" (credentials come from sts:AssumeRole against role_arns[0]).",
			},
			"policy_arns": schema.ListAttribute{
				Optional:    true,
				Computed:    true,
				ElementType: types.StringType,
				Description: "Managed policy ARNs attached to generated iam_user credentials.",
			},
			"policy_document": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Inline IAM policy JSON attached to generated iam_user credentials.",
			},
			"role_arns": schema.ListAttribute{
				Optional:    true,
				Computed:    true,
				ElementType: types.StringType,
				Description: "Role ARNs for assumed_role credentials — the first entry is the one assumed.",
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

type awsRoleModel struct {
	Name           types.String `tfsdk:"name"`
	CredentialType types.String `tfsdk:"credential_type"`
	PolicyARNs     types.List   `tfsdk:"policy_arns"`
	PolicyDocument types.String `tfsdk:"policy_document"`
	RoleARNs       types.List   `tfsdk:"role_arns"`
	DefaultTTL     types.String `tfsdk:"default_ttl"`
	MaxTTL         types.String `tfsdk:"max_ttl"`
}

func (r *AWSRoleResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

// writeAndRead does the PUT-then-GET round trip every AWS/GCP/Azure role
// write needs, since their PUT endpoints respond 204 with no body (unlike
// PKI/SSH/database roles, which return the stored role directly).
func (r *AWSRoleResource) writeAndRead(ctx context.Context, name string, apiReq awsRoleReq) (*awsRoleAPIResp, error) {
	if err := r.client.putAWSRole(ctx, name, apiReq); err != nil {
		return nil, err
	}
	role, found, err := r.client.getAWSRole(ctx, name)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, errRoleDisappearedAfterWrite(name)
	}
	return role, nil
}

func (r *AWSRoleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan awsRoleModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	result, err := r.writeAndRead(ctx, plan.Name.ValueString(), awsRoleModelToReq(ctx, plan))
	if err != nil {
		resp.Diagnostics.AddError("Error creating AWS role", err.Error())
		return
	}
	state, diags := awsRoleRespToModel(ctx, result)
	preservePlannedTTLs(&state.DefaultTTL, &state.MaxTTL, plan.DefaultTTL, plan.MaxTTL)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *AWSRoleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state awsRoleModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	role, found, err := r.client.getAWSRole(ctx, state.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading AWS role", err.Error())
		return
	}
	if !found {
		resp.State.RemoveResource(ctx)
		return
	}
	newState, diags := awsRoleRespToModel(ctx, role)
	newState.DefaultTTL = types.StringValue(preserveEquivalentTTL(newState.DefaultTTL.ValueString(), state.DefaultTTL))
	newState.MaxTTL = types.StringValue(preserveEquivalentTTL(newState.MaxTTL.ValueString(), state.MaxTTL))
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, newState)...)
}

func (r *AWSRoleResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan awsRoleModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	result, err := r.writeAndRead(ctx, plan.Name.ValueString(), awsRoleModelToReq(ctx, plan))
	if err != nil {
		resp.Diagnostics.AddError("Error updating AWS role", err.Error())
		return
	}
	state, diags := awsRoleRespToModel(ctx, result)
	preservePlannedTTLs(&state.DefaultTTL, &state.MaxTTL, plan.DefaultTTL, plan.MaxTTL)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *AWSRoleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state awsRoleModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.deleteAWSRole(ctx, state.Name.ValueString()); err != nil {
		resp.Diagnostics.AddError("Error deleting AWS role", err.Error())
	}
}

func awsRoleModelToReq(ctx context.Context, m awsRoleModel) awsRoleReq {
	var policyARNs, roleARNs []string
	_ = m.PolicyARNs.ElementsAs(ctx, &policyARNs, false)
	_ = m.RoleARNs.ElementsAs(ctx, &roleARNs, false)
	return awsRoleReq{
		CredentialType: m.CredentialType.ValueString(),
		PolicyARNs:     policyARNs,
		PolicyDocument: m.PolicyDocument.ValueString(),
		RoleARNs:       roleARNs,
		DefaultTTL:     m.DefaultTTL.ValueString(),
		MaxTTL:         m.MaxTTL.ValueString(),
	}
}

func awsRoleRespToModel(ctx context.Context, role *awsRoleAPIResp) (awsRoleModel, diag.Diagnostics) {
	var diags diag.Diagnostics
	policyARNs, d := types.ListValueFrom(ctx, types.StringType, role.PolicyARNs)
	diags.Append(d...)
	roleARNs, d := types.ListValueFrom(ctx, types.StringType, role.RoleARNs)
	diags.Append(d...)
	return awsRoleModel{
		Name:           types.StringValue(role.Name),
		CredentialType: types.StringValue(role.CredentialType),
		PolicyARNs:     policyARNs,
		PolicyDocument: types.StringValue(role.PolicyDocument),
		RoleARNs:       roleARNs,
		DefaultTTL:     types.StringValue(nsDuration(role.DefaultTTL)),
		MaxTTL:         types.StringValue(nsDuration(role.MaxTTL)),
	}, diags
}
