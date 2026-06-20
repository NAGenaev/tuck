package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &MountResource{}

// MountResource manages a Tuck secret engine mount.
type MountResource struct {
	client *tuckClient
}

func NewMountResource() resource.Resource {
	return &MountResource{}
}

func (r *MountResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_mount"
}

func (r *MountResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Mounts a secret engine at a given path. Both path and type are immutable; changing either forces replacement.",
		Attributes: map[string]schema.Attribute{
			"path": schema.StringAttribute{
				Required:    true,
				Description: "Mount path (e.g. \"transit/\", \"pki/\"). Must end with /.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"type": schema.StringAttribute{
				Required:    true,
				Description: "Engine type: kv, transit, pki, database, aws, gcp, azure, ssh, totp.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"description": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Human-readable description for the mount.",
			},
			"accessor": schema.StringAttribute{
				Computed:    true,
				Description: "Server-assigned accessor for this mount.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

type mountModel struct {
	Path        types.String `tfsdk:"path"`
	Type        types.String `tfsdk:"type"`
	Description types.String `tfsdk:"description"`
	Accessor    types.String `tfsdk:"accessor"`
}

func (r *MountResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *MountResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan mountModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.createMount(ctx, plan.Path.ValueString(), plan.Type.ValueString(), plan.Description.ValueString()); err != nil {
		resp.Diagnostics.AddError("Error creating mount", err.Error())
		return
	}
	// Read back to get accessor.
	m, found, err := r.client.getMount(ctx, plan.Path.ValueString())
	if err != nil || !found {
		resp.Diagnostics.AddError("Error reading mount after create", fmt.Sprintf("found=%v err=%v", found, err))
		return
	}
	plan.Accessor = types.StringValue(m.Accessor)
	plan.Description = types.StringValue(m.Description)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *MountResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state mountModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	m, found, err := r.client.getMount(ctx, state.Path.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading mount", err.Error())
		return
	}
	if !found {
		resp.State.RemoveResource(ctx)
		return
	}
	state.Accessor = types.StringValue(m.Accessor)
	state.Description = types.StringValue(m.Description)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *MountResource) Update(_ context.Context, _ resource.UpdateRequest, _ *resource.UpdateResponse) {
	// path and type are ForceNew; description has no server-side update endpoint — no-op.
}

func (r *MountResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state mountModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.deleteMount(ctx, state.Path.ValueString()); err != nil {
		resp.Diagnostics.AddError("Error deleting mount", err.Error())
	}
}

// ─── tuckClient helpers ───────────────────────────────────────────────────────

type mountInfo struct {
	Path        string `json:"path"`
	Type        string `json:"type"`
	Description string `json:"description"`
	Accessor    string `json:"accessor"`
	Builtin     bool   `json:"builtin"`
}

func (c *tuckClient) createMount(ctx context.Context, path, engineType, description string) error {
	_, status, err := c.doJSON(ctx, http.MethodPost, "/v1/sys/mounts/"+path, map[string]string{
		"type":        engineType,
		"description": description,
	})
	if err != nil {
		return err
	}
	if status != http.StatusNoContent && status != http.StatusOK && status != http.StatusCreated {
		return fmt.Errorf("tuck POST mounts/%s: HTTP %d", path, status)
	}
	return nil
}

func (c *tuckClient) getMount(ctx context.Context, path string) (*mountInfo, bool, error) {
	body, status, err := c.doJSON(ctx, http.MethodGet, "/v1/sys/mounts", nil)
	if err != nil {
		return nil, false, err
	}
	if status != http.StatusOK {
		return nil, false, fmt.Errorf("tuck GET mounts: HTTP %d", status)
	}
	var resp struct {
		Mounts []mountInfo `json:"mounts"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, false, fmt.Errorf("parse mounts response: %w", err)
	}
	for _, m := range resp.Mounts {
		if m.Path == path {
			return &m, true, nil
		}
	}
	return nil, false, nil
}

func (c *tuckClient) deleteMount(ctx context.Context, path string) error {
	_, status, err := c.doJSON(ctx, http.MethodDelete, "/v1/sys/mounts/"+path, nil)
	if err != nil {
		return err
	}
	if status != http.StatusNoContent && status != http.StatusNotFound {
		return fmt.Errorf("tuck DELETE mounts/%s: HTTP %d", path, status)
	}
	return nil
}
