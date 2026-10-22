package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/int32validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/resourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int32planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/objectplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/lannuttia/terraform-provider-quay/internal/client"
)

var _ resource.Resource = &RepositoryNotificationResource{}
var _ resource.ResourceWithImportState = &RepositoryNotificationResource{}
var _ resource.ResourceWithConfigValidators = &RepositoryNotificationResource{}

type RepositoryNotificationResource struct {
	client *client.Client
}

// -------------------------------------------------------------------------
// Terraform state model
// -------------------------------------------------------------------------

type RepositoryNotificationResourceModel struct {
	ID               types.String `tfsdk:"id"`
	Repository       types.String `tfsdk:"repository"`
	UUID             types.String `tfsdk:"uuid"`
	Title            types.String `tfsdk:"title"`
	NumberOfFailures types.Int64  `tfsdk:"number_of_failures"`

	// Exactly one method block must be set.
	Webhook          *WebhookConfig          `tfsdk:"webhook"`
	Slack            *SlackConfig            `tfsdk:"slack"`
	Email            *EmailConfig            `tfsdk:"email"`
	Hipchat          *HipchatConfig          `tfsdk:"hipchat"`
	Flowdock         *FlowdockConfig         `tfsdk:"flowdock"`
	QuayNotification *QuayNotificationConfig `tfsdk:"quay_notification"`

	// Exactly one event block must be set. Event-specific config lives inside
	// each block, making invalid combinations structurally impossible.
	RepoPush              *RepoPushEvent              `tfsdk:"repo_push"`
	BuildQueued           *BuildEvent                 `tfsdk:"build_queued"`
	BuildStart            *BuildEvent                 `tfsdk:"build_start"`
	BuildSuccess          *BuildEvent                 `tfsdk:"build_success"`
	BuildFailure          *BuildEvent                 `tfsdk:"build_failure"`
	BuildCancelled        *BuildEvent                 `tfsdk:"build_cancelled"`
	VulnerabilityFound    *VulnerabilityFoundEvent    `tfsdk:"vulnerability_found"`
	RepoMirrorSyncStarted *RepoMirrorSyncStartedEvent `tfsdk:"repo_mirror_sync_started"`
	RepoMirrorSyncSuccess *RepoMirrorSyncSuccessEvent `tfsdk:"repo_mirror_sync_success"`
	RepoMirrorSyncFailed  *RepoMirrorSyncFailedEvent  `tfsdk:"repo_mirror_sync_failed"`
}

// Method config types

type WebhookConfig struct {
	URL types.String `tfsdk:"url"`
}

type SlackConfig struct {
	URL types.String `tfsdk:"url"`
}

type EmailConfig struct {
	Email types.String `tfsdk:"email"`
}

type HipchatConfig struct {
	NotificationToken types.String `tfsdk:"notification_token"`
	RoomID            types.String `tfsdk:"room_id"`
}

type FlowdockConfig struct {
	FlowAPIToken types.String `tfsdk:"flow_api_token"`
}

// QuayNotificationTarget identifies the Quay user or team to notify.
// Quay's API requires this inside config.target; without it the API returns
// 400 "Missing target".
type QuayNotificationTarget struct {
	Kind types.String `tfsdk:"kind"`
	Name types.String `tfsdk:"name"`
}

type QuayNotificationConfig struct {
	Target QuayNotificationTarget `tfsdk:"target"`
}

// Event types
// Events with no configuration use an empty struct so the block is still
// present in the schema and self-documents that no options exist.

type RepoPushEvent struct{}

type BuildEvent struct {
	RefRegex types.String `tfsdk:"ref_regex"`
}

type VulnerabilityFoundEvent struct {
	Level types.Int32 `tfsdk:"level"`
}

type RepoMirrorSyncStartedEvent struct{}
type RepoMirrorSyncSuccessEvent struct{}
type RepoMirrorSyncFailedEvent struct{}

// -------------------------------------------------------------------------
// Constructor / metadata
// -------------------------------------------------------------------------

func NewRepositoryNotificationResource() resource.Resource {
	return &RepositoryNotificationResource{}
}

func (r *RepositoryNotificationResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_repository_notification"
}

// -------------------------------------------------------------------------
// Schema
// -------------------------------------------------------------------------

// buildEventAttributes returns the shared attribute set for all build event blocks.
func buildEventAttributes(requiresReplace []planmodifier.String) map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"ref_regex": schema.StringAttribute{
			Optional:      true,
			Description:   "A regular expression that Git refs must match to trigger the notification.",
			PlanModifiers: requiresReplace,
		},
	}
}

func (r *RepositoryNotificationResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	requiresReplaceStr := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	requiresReplaceObj := []planmodifier.Object{objectplanmodifier.RequiresReplace()}

	resp.Schema = schema.Schema{
		Description: "Manages a notification on a Quay repository. " +
			"Because the Quay API has no update endpoint for notifications, any change to notification " +
			"attributes will cause the existing notification to be deleted and a new one created. " +
			"Exactly one method block (webhook, slack, email, hipchat, flowdock, or quay_notification) " +
			"must be provided. Exactly one event block must be provided; event-specific configuration " +
			"(such as the vulnerability severity level or a build ref regex) lives inside that block.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "The resource identifier (repository/uuid).",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"repository": schema.StringAttribute{
				Required:      true,
				Description:   "The repository this notification belongs to, in namespace/name format.",
				PlanModifiers: requiresReplaceStr,
			},
			"uuid": schema.StringAttribute{
				Computed:    true,
				Description: "The server-assigned UUID of the notification.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"title": schema.StringAttribute{
				Optional:      true,
				Computed:      true,
				Description:   "A human-readable title for the notification.",
				PlanModifiers: append(requiresReplaceStr, stringplanmodifier.UseStateForUnknown()),
			},
			"number_of_failures": schema.Int64Attribute{
				Computed:    true,
				Description: "The number of consecutive delivery failures.",
			},

			// ----- method blocks -----

			"webhook": schema.SingleNestedAttribute{
				Optional:      true,
				Description:   "Deliver notifications via an HTTP POST to a URL.",
				PlanModifiers: requiresReplaceObj,
				Attributes: map[string]schema.Attribute{
					"url": schema.StringAttribute{
						Required:      true,
						Sensitive:     true,
						Description:   "The URL to POST the notification payload to. Marked sensitive because webhook URLs frequently embed authentication tokens.",
						PlanModifiers: requiresReplaceStr,
					},
				},
			},
			"slack": schema.SingleNestedAttribute{
				Optional:      true,
				Description:   "Deliver notifications to a Slack channel via an Incoming Webhook URL.",
				PlanModifiers: requiresReplaceObj,
				Attributes: map[string]schema.Attribute{
					"url": schema.StringAttribute{
						Required:      true,
						Sensitive:     true,
						Description:   "The Slack Incoming Webhook URL. Marked sensitive because the URL itself is the credential.",
						PlanModifiers: requiresReplaceStr,
					},
				},
			},
			"email": schema.SingleNestedAttribute{
				Optional:      true,
				Description:   "Deliver notifications by email.",
				PlanModifiers: requiresReplaceObj,
				Attributes: map[string]schema.Attribute{
					"email": schema.StringAttribute{
						Required:      true,
						Description:   "The recipient email address.",
						PlanModifiers: requiresReplaceStr,
					},
				},
			},
			"hipchat": schema.SingleNestedAttribute{
				Optional:      true,
				Description:   "Deliver notifications to a HipChat room.",
				PlanModifiers: requiresReplaceObj,
				Attributes: map[string]schema.Attribute{
					"notification_token": schema.StringAttribute{
						Required:      true,
						Sensitive:     true,
						Description:   "The HipChat API notification token.",
						PlanModifiers: requiresReplaceStr,
					},
					"room_id": schema.StringAttribute{
						Required:      true,
						Description:   "The HipChat room ID.",
						PlanModifiers: requiresReplaceStr,
					},
				},
			},
			"flowdock": schema.SingleNestedAttribute{
				Optional:      true,
				Description:   "Deliver notifications to a Flowdock flow.",
				PlanModifiers: requiresReplaceObj,
				Attributes: map[string]schema.Attribute{
					"flow_api_token": schema.StringAttribute{
						Required:      true,
						Sensitive:     true,
						Description:   "The Flowdock flow API token.",
						PlanModifiers: requiresReplaceStr,
					},
				},
			},
			"quay_notification": schema.SingleNestedAttribute{
				Optional:      true,
				Description:   "Deliver notifications via the built-in Quay notification system.",
				PlanModifiers: requiresReplaceObj,
				Attributes: map[string]schema.Attribute{
					"target": schema.SingleNestedAttribute{
						Required:      true,
						Description:   "The Quay user or team to notify.",
						PlanModifiers: requiresReplaceObj,
						Attributes: map[string]schema.Attribute{
							"kind": schema.StringAttribute{
								Required:      true,
								Description:   `The kind of target. One of: "user" or "team".`,
								PlanModifiers: requiresReplaceStr,
							},
							"name": schema.StringAttribute{
								Required:      true,
								Description:   "The username or team name to notify.",
								PlanModifiers: requiresReplaceStr,
							},
						},
					},
				},
			},

			// ----- event blocks -----
			// Exactly one must be set (enforced by ConfigValidators).
			// Event-specific config is nested inside the relevant block,
			// making it impossible to supply config for the wrong event type.

			"repo_push": schema.SingleNestedAttribute{
				Optional:      true,
				Description:   "Trigger on every image push to the repository.",
				PlanModifiers: requiresReplaceObj,
				Attributes:    map[string]schema.Attribute{},
			},
			"build_queued": schema.SingleNestedAttribute{
				Optional:      true,
				Description:   "Trigger when a build is queued.",
				PlanModifiers: requiresReplaceObj,
				Attributes:    buildEventAttributes(requiresReplaceStr),
			},
			"build_start": schema.SingleNestedAttribute{
				Optional:      true,
				Description:   "Trigger when a build starts.",
				PlanModifiers: requiresReplaceObj,
				Attributes:    buildEventAttributes(requiresReplaceStr),
			},
			"build_success": schema.SingleNestedAttribute{
				Optional:      true,
				Description:   "Trigger when a build succeeds.",
				PlanModifiers: requiresReplaceObj,
				Attributes:    buildEventAttributes(requiresReplaceStr),
			},
			"build_failure": schema.SingleNestedAttribute{
				Optional:      true,
				Description:   "Trigger when a build fails.",
				PlanModifiers: requiresReplaceObj,
				Attributes:    buildEventAttributes(requiresReplaceStr),
			},
			"build_cancelled": schema.SingleNestedAttribute{
				Optional:      true,
				Description:   "Trigger when a build is cancelled.",
				PlanModifiers: requiresReplaceObj,
				Attributes:    buildEventAttributes(requiresReplaceStr),
			},
			"vulnerability_found": schema.SingleNestedAttribute{
				Optional:      true,
				Description:   "Trigger when a vulnerability is found. The required level attribute sets the minimum severity that triggers the notification.",
				PlanModifiers: requiresReplaceObj,
				Attributes: map[string]schema.Attribute{
					"level": schema.Int32Attribute{
						Required:      true,
						Description:   "The minimum vulnerability severity level index that triggers the notification (0=Critical, 1=High, 2=Medium, 3=Low, 4=Negligible, 5=Unknown).",
						Validators:    []validator.Int32{int32validator.Between(0, 5)},
						PlanModifiers: []planmodifier.Int32{int32planmodifier.RequiresReplace()},
					},
				},
			},
			"repo_mirror_sync_started": schema.SingleNestedAttribute{
				Optional:      true,
				Description:   "Trigger when a repository mirror sync starts.",
				PlanModifiers: requiresReplaceObj,
				Attributes:    map[string]schema.Attribute{},
			},
			"repo_mirror_sync_success": schema.SingleNestedAttribute{
				Optional:      true,
				Description:   "Trigger when a repository mirror sync succeeds.",
				PlanModifiers: requiresReplaceObj,
				Attributes:    map[string]schema.Attribute{},
			},
			"repo_mirror_sync_failed": schema.SingleNestedAttribute{
				Optional:      true,
				Description:   "Trigger when a repository mirror sync fails.",
				PlanModifiers: requiresReplaceObj,
				Attributes:    map[string]schema.Attribute{},
			},
		},
	}
}

// ConfigValidators enforces that exactly one method block and exactly one
// event block are set.
func (r *RepositoryNotificationResource) ConfigValidators(_ context.Context) []resource.ConfigValidator {
	return []resource.ConfigValidator{
		resourcevalidator.ExactlyOneOf(
			path.MatchRoot("webhook"),
			path.MatchRoot("slack"),
			path.MatchRoot("email"),
			path.MatchRoot("hipchat"),
			path.MatchRoot("flowdock"),
			path.MatchRoot("quay_notification"),
		),
		resourcevalidator.ExactlyOneOf(
			path.MatchRoot("repo_push"),
			path.MatchRoot("build_queued"),
			path.MatchRoot("build_start"),
			path.MatchRoot("build_success"),
			path.MatchRoot("build_failure"),
			path.MatchRoot("build_cancelled"),
			path.MatchRoot("vulnerability_found"),
			path.MatchRoot("repo_mirror_sync_started"),
			path.MatchRoot("repo_mirror_sync_success"),
			path.MatchRoot("repo_mirror_sync_failed"),
		),
	}
}

// -------------------------------------------------------------------------
// CRUD
// -------------------------------------------------------------------------

func (r *RepositoryNotificationResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected Provider Data", fmt.Sprintf("Expected *client.Client, got %T", req.ProviderData))
		return
	}
	r.client = c
}

func (r *RepositoryNotificationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan RepositoryNotificationResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	method, methodConfig := plan.methodAndConfig()
	event, eventConfig := plan.eventAndConfig()

	notif, err := r.client.CreateRepoNotification(ctx, plan.Repository.ValueString(), client.CreateNotificationRequest{
		Event:       event,
		Method:      method,
		Config:      methodConfig,
		EventConfig: eventConfig,
		Title:       plan.Title.ValueString(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Error Creating Repository Notification", err.Error())
		return
	}

	resp.Diagnostics.Append(r.mapToState(notif, plan.Repository.ValueString(), &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *RepositoryNotificationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state RepositoryNotificationResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	notif, err := r.client.GetRepoNotification(ctx, state.Repository.ValueString(), state.UUID.ValueString())
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error Reading Repository Notification", err.Error())
		return
	}

	resp.Diagnostics.Append(r.mapToState(notif, state.Repository.ValueString(), &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *RepositoryNotificationResource) Update(_ context.Context, _ resource.UpdateRequest, _ *resource.UpdateResponse) {
	// All meaningful fields use RequiresReplace — update is never called.
}

func (r *RepositoryNotificationResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state RepositoryNotificationResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.DeleteRepoNotification(ctx, state.Repository.ValueString(), state.UUID.ValueString())
	if err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error Deleting Repository Notification", err.Error())
	}
}

func (r *RepositoryNotificationResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Import ID: namespace/repo_name/uuid  (3 parts, last part is the UUID)
	parts, diags := splitImportID(resp, req.ID, 3, "my-org/my-repo/32bb0ef6-ad69-4ccb-b64e-ddbc8bfd444d")
	if diags.HasError() {
		return
	}
	repository := parts[0] + "/" + parts[1]
	setAttrs(ctx, resp, map[string]string{
		"id":         req.ID,
		"repository": repository,
		"uuid":       parts[2],
	})
}

// -------------------------------------------------------------------------
// Mapping helpers
// -------------------------------------------------------------------------

// methodAndConfig derives the API method string and config map from whichever
// method block is set in the plan.
func (m *RepositoryNotificationResourceModel) methodAndConfig() (string, map[string]any) {
	switch {
	case m.Webhook != nil:
		return "webhook", map[string]any{"url": m.Webhook.URL.ValueString()}
	case m.Slack != nil:
		return "slack", map[string]any{"url": m.Slack.URL.ValueString()}
	case m.Email != nil:
		return "email", map[string]any{"email": m.Email.Email.ValueString()}
	case m.Hipchat != nil:
		return "hipchat", map[string]any{
			"notification_token": m.Hipchat.NotificationToken.ValueString(),
			"room_id":            m.Hipchat.RoomID.ValueString(),
		}
	case m.Flowdock != nil:
		return "flowdock", map[string]any{"flow_api_token": m.Flowdock.FlowAPIToken.ValueString()}
	case m.QuayNotification != nil:
		return "quay_notification", map[string]any{
			"target": map[string]any{
				"kind": m.QuayNotification.Target.Kind.ValueString(),
				"name": m.QuayNotification.Target.Name.ValueString(),
			},
		}
	default:
		// ConfigValidators guarantees this is unreachable.
		return "", map[string]any{}
	}
}

// eventAndConfig derives the API event string and event_config map from
// whichever event block is set in the plan.
func (m *RepositoryNotificationResourceModel) eventAndConfig() (string, map[string]any) {
	switch {
	case m.RepoPush != nil:
		return "repo_push", map[string]any{}
	case m.BuildQueued != nil:
		return "build_queued", buildRefRegexConfig(m.BuildQueued.RefRegex)
	case m.BuildStart != nil:
		return "build_start", buildRefRegexConfig(m.BuildStart.RefRegex)
	case m.BuildSuccess != nil:
		return "build_success", buildRefRegexConfig(m.BuildSuccess.RefRegex)
	case m.BuildFailure != nil:
		return "build_failure", buildRefRegexConfig(m.BuildFailure.RefRegex)
	case m.BuildCancelled != nil:
		return "build_cancelled", buildRefRegexConfig(m.BuildCancelled.RefRegex)
	case m.VulnerabilityFound != nil:
		return "vulnerability_found", map[string]any{"level": m.VulnerabilityFound.Level.ValueInt32()}
	case m.RepoMirrorSyncStarted != nil:
		return "repo_mirror_sync_started", map[string]any{}
	case m.RepoMirrorSyncSuccess != nil:
		return "repo_mirror_sync_success", map[string]any{}
	case m.RepoMirrorSyncFailed != nil:
		return "repo_mirror_sync_failed", map[string]any{}
	default:
		// ConfigValidators guarantees this is unreachable.
		return "", map[string]any{}
	}
}

// buildRefRegexConfig returns an event_config map for build events,
// omitting the ref-regex key when the value is null or unknown.
func buildRefRegexConfig(refRegex types.String) map[string]any {
	cfg := map[string]any{}
	if !refRegex.IsNull() && !refRegex.IsUnknown() {
		cfg["ref-regex"] = refRegex.ValueString()
	}
	return cfg
}

// mapToState populates the model from a Notification API response.
func (r *RepositoryNotificationResource) mapToState(notif *client.Notification, repository string, state *RepositoryNotificationResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics

	state.Repository = types.StringValue(repository)
	state.UUID = types.StringValue(notif.UUID)
	state.Title = types.StringValue(notif.Title)
	state.NumberOfFailures = types.Int64Value(int64(notif.NumberOfFailures))
	state.ID = types.StringValue(fmt.Sprintf("%s/%s", repository, notif.UUID))

	// Clear all method blocks, then populate only the one that matches.
	state.Webhook = nil
	state.Slack = nil
	state.Email = nil
	state.Hipchat = nil
	state.Flowdock = nil
	state.QuayNotification = nil

	switch notif.Method {
	case "webhook":
		state.Webhook = &WebhookConfig{URL: types.StringValue(stringFromConfig(notif.Config, "url"))}
	case "slack":
		state.Slack = &SlackConfig{URL: types.StringValue(stringFromConfig(notif.Config, "url"))}
	case "email":
		state.Email = &EmailConfig{Email: types.StringValue(stringFromConfig(notif.Config, "email"))}
	case "hipchat":
		state.Hipchat = &HipchatConfig{
			NotificationToken: types.StringValue(stringFromConfig(notif.Config, "notification_token")),
			RoomID:            types.StringValue(stringFromConfig(notif.Config, "room_id")),
		}
	case "flowdock":
		state.Flowdock = &FlowdockConfig{FlowAPIToken: types.StringValue(stringFromConfig(notif.Config, "flow_api_token"))}
	case "quay_notification":
		target, _ := notif.Config["target"].(map[string]any)
		kind := ""
		name := ""
		if target != nil {
			kind = fmt.Sprintf("%v", target["kind"])
			name = fmt.Sprintf("%v", target["name"])
		}
		state.QuayNotification = &QuayNotificationConfig{
			Target: QuayNotificationTarget{
				Kind: types.StringValue(kind),
				Name: types.StringValue(name),
			},
		}
	default:
		diags.AddError("Unknown Notification Method",
			fmt.Sprintf("The Quay API returned an unrecognised notification method %q. "+
				"Please report this as a provider bug.", notif.Method))
		return diags
	}

	// Clear all event blocks, then populate only the one that matches.
	state.RepoPush = nil
	state.BuildQueued = nil
	state.BuildStart = nil
	state.BuildSuccess = nil
	state.BuildFailure = nil
	state.BuildCancelled = nil
	state.VulnerabilityFound = nil
	state.RepoMirrorSyncStarted = nil
	state.RepoMirrorSyncSuccess = nil
	state.RepoMirrorSyncFailed = nil

	switch notif.Event {
	case "repo_push":
		state.RepoPush = &RepoPushEvent{}
	case "build_queued":
		state.BuildQueued = &BuildEvent{RefRegex: refRegexFromEventConfig(notif.EventConfig)}
	case "build_start":
		state.BuildStart = &BuildEvent{RefRegex: refRegexFromEventConfig(notif.EventConfig)}
	case "build_success":
		state.BuildSuccess = &BuildEvent{RefRegex: refRegexFromEventConfig(notif.EventConfig)}
	case "build_failure":
		state.BuildFailure = &BuildEvent{RefRegex: refRegexFromEventConfig(notif.EventConfig)}
	case "build_cancelled":
		state.BuildCancelled = &BuildEvent{RefRegex: refRegexFromEventConfig(notif.EventConfig)}
	case "vulnerability_found":
		level, _ := notif.EventConfig.VulnerabilityLevelInt32()
		state.VulnerabilityFound = &VulnerabilityFoundEvent{
			Level: types.Int32Value(level),
		}
	case "repo_mirror_sync_started":
		state.RepoMirrorSyncStarted = &RepoMirrorSyncStartedEvent{}
	case "repo_mirror_sync_success":
		state.RepoMirrorSyncSuccess = &RepoMirrorSyncSuccessEvent{}
	case "repo_mirror_sync_failed":
		state.RepoMirrorSyncFailed = &RepoMirrorSyncFailedEvent{}
	default:
		diags.AddError("Unknown Notification Event",
			fmt.Sprintf("The Quay API returned an unrecognised notification event %q. "+
				"Please report this as a provider bug.", notif.Event))
		return diags
	}

	return diags
}

// refRegexFromEventConfig extracts the ref-regex value from a build event
// config, returning a null string when the key is absent.
func refRegexFromEventConfig(eventConfig client.EventConfig) types.String {
	if v, ok := eventConfig["ref-regex"]; ok {
		return types.StringValue(fmt.Sprintf("%v", v))
	}
	return types.StringNull()
}

// stringFromConfig safely extracts a string value from a map[string]any.
func stringFromConfig(m map[string]any, key string) string {
	if v, ok := m[key]; ok {
		return fmt.Sprintf("%v", v)
	}
	return ""
}
