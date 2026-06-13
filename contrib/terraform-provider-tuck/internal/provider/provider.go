package provider

import (
	"context"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ provider.Provider = &TuckProvider{}

// TuckProvider is the Terraform provider for the Tuck secrets manager.
type TuckProvider struct {
	version string
}

// New returns a provider factory for the given version string.
func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &TuckProvider{version: version}
	}
}

func (p *TuckProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "tuck"
	resp.Version = p.version
}

func (p *TuckProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "The Tuck provider manages secrets and policies in a Tuck secrets manager instance.",
		Attributes: map[string]schema.Attribute{
			"addr": schema.StringAttribute{
				Description: "Tuck server address (e.g. \"https://tuck:8200\"). Overrides TUCK_ADDR env var.",
				Optional:    true,
			},
			"token": schema.StringAttribute{
				Description: "Tuck auth token. Overrides TUCK_TOKEN env var.",
				Optional:    true,
				Sensitive:   true,
			},
			"namespace": schema.StringAttribute{
				Description: "Tuck namespace (empty = root). Overrides TUCK_NAMESPACE env var.",
				Optional:    true,
			},
			"insecure": schema.BoolAttribute{
				Description: "Skip TLS certificate verification. Use only in development.",
				Optional:    true,
			},
		},
	}
}

// tuckProviderModel maps the provider schema to Go types.
type tuckProviderModel struct {
	Addr      types.String `tfsdk:"addr"`
	Token     types.String `tfsdk:"token"`
	Namespace types.String `tfsdk:"namespace"`
	Insecure  types.Bool   `tfsdk:"insecure"`
}

func (p *TuckProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var config tuckProviderModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	addr := envOr("TUCK_ADDR", "http://127.0.0.1:8200")
	if !config.Addr.IsNull() && !config.Addr.IsUnknown() {
		addr = config.Addr.ValueString()
	}

	token := os.Getenv("TUCK_TOKEN")
	if !config.Token.IsNull() && !config.Token.IsUnknown() {
		token = config.Token.ValueString()
	}

	namespace := os.Getenv("TUCK_NAMESPACE")
	if !config.Namespace.IsNull() && !config.Namespace.IsUnknown() {
		namespace = config.Namespace.ValueString()
	}

	insecure := false
	if !config.Insecure.IsNull() && !config.Insecure.IsUnknown() {
		insecure = config.Insecure.ValueBool()
	}

	client := newTuckClient(addr, token, namespace, insecure)
	resp.DataSourceData = client
	resp.ResourceData = client
}

func (p *TuckProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewKVSecretResource,
		NewPolicyResource,
	}
}

func (p *TuckProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewKVSecretDataSource,
	}
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
