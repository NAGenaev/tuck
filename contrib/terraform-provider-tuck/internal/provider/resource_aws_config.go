package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &AWSConfigResource{}

// AWSConfigResource manages Tuck's AWS dynamic-secrets engine config. This
// is a singleton on the server (PUT/GET/DELETE /v1/aws/config, no name in
// the path) — the Terraform resource address itself gives it identity, so
// the schema carries no id/name attribute.
type AWSConfigResource struct {
	client *tuckClient
}

func NewAWSConfigResource() resource.Resource {
	return &AWSConfigResource{}
}

func (r *AWSConfigResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_aws_config"
}

func (r *AWSConfigResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages Tuck's AWS dynamic-secrets engine configuration (singleton — one per Tuck server/namespace).",
		Attributes: map[string]schema.Attribute{
			"access_key_id": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Root AWS access key ID used to create/revoke dynamic IAM credentials. Omit to use the AWS SDK's default credential chain (instance role, env vars, etc.).",
			},
			"secret_access_key": schema.StringAttribute{
				Optional:    true,
				Sensitive:   true,
				Description: "Root AWS secret access key. Tuck never returns this value back on read (the API redacts it), so Terraform always trusts the configured value rather than server state for this attribute.",
			},
			"region": schema.StringAttribute{
				Required:    true,
				Description: "AWS region for IAM/STS API calls.",
			},
			"iam_endpoint": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Override the IAM API endpoint (dev/test only).",
			},
			"sts_endpoint": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Override the STS API endpoint (dev/test only).",
			},
		},
	}
}

type awsConfigModel struct {
	AccessKeyID     types.String `tfsdk:"access_key_id"`
	SecretAccessKey types.String `tfsdk:"secret_access_key"`
	Region          types.String `tfsdk:"region"`
	IAMEndpoint     types.String `tfsdk:"iam_endpoint"`
	STSEndpoint     types.String `tfsdk:"sts_endpoint"`
}

func (r *AWSConfigResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *AWSConfigResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan awsConfigModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.putAWSConfig(ctx, awsConfigModelToReq(plan)); err != nil {
		resp.Diagnostics.AddError("Error creating AWS config", err.Error())
		return
	}
	// Nothing is server-computed for this resource — the plan is already
	// the complete, final state.
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *AWSConfigResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state awsConfigModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	cfg, found, err := r.client.getAWSConfig(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Error reading AWS config", err.Error())
		return
	}
	if !found {
		resp.State.RemoveResource(ctx)
		return
	}
	newState := awsConfigModel{
		AccessKeyID: types.StringValue(cfg.AccessKeyID),
		// secret_access_key comes back blanked from the API — keep whatever
		// Terraform already has, never overwrite it from a read.
		SecretAccessKey: state.SecretAccessKey,
		Region:          types.StringValue(cfg.Region),
		IAMEndpoint:     types.StringValue(cfg.IAMEndpoint),
		STSEndpoint:     types.StringValue(cfg.STSEndpoint),
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, newState)...)
}

func (r *AWSConfigResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan awsConfigModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.putAWSConfig(ctx, awsConfigModelToReq(plan)); err != nil {
		resp.Diagnostics.AddError("Error updating AWS config", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *AWSConfigResource) Delete(ctx context.Context, _ resource.DeleteRequest, resp *resource.DeleteResponse) {
	if err := r.client.deleteAWSConfig(ctx); err != nil {
		resp.Diagnostics.AddError("Error deleting AWS config", err.Error())
	}
}

func awsConfigModelToReq(m awsConfigModel) awsConfigReq {
	return awsConfigReq{
		AccessKeyID:     m.AccessKeyID.ValueString(),
		SecretAccessKey: m.SecretAccessKey.ValueString(),
		Region:          m.Region.ValueString(),
		IAMEndpoint:     m.IAMEndpoint.ValueString(),
		STSEndpoint:     m.STSEndpoint.ValueString(),
	}
}
