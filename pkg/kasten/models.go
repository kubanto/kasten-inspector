package kasten

// ── Top-level ─────────────────────────────────────────────────────────────────

type CollectOptions struct {
	Namespace string
	JobLimit  int
	Verbose   bool
}

type Data struct {
	Version        string             `json:"kastenVersion"`
	HelmConfig     HelmConfig         `json:"helmConfiguration"`
	License        License            `json:"license"`
	MultiCluster   MultiClusterInfo   `json:"multiCluster"`
	DR             DRInfo             `json:"disasterRecovery"`
	Security       SecurityConfig     `json:"security"`
	Applications   AppSummary         `json:"applications"`
	Namespaces     NamespaceSummary   `json:"namespaces"`
	Policies       []Policy           `json:"policies"`
	PolicyPresets  []PolicyPreset     `json:"policyPresets"`
	Profiles       []Profile          `json:"profiles"`
	Jobs           []Job              `json:"recentJobs"`
	JobSummary     JobSummary         `json:"jobSummary"`
	RestorePoints  RestorePointInfo   `json:"restorePoints"`
	KubeVirt       KubeVirtInfo       `json:"kubeVirt"`
	Blueprints     []Blueprint        `json:"kanisterBlueprints"`
	Bindings       []BlueprintBinding `json:"blueprintBindings"`
	TransformSets  []TransformSet     `json:"transformSets"`
	Resources      K10Resources       `json:"k10Resources"`
	Catalog        CatalogInfo        `json:"catalog"`
	Storage        StorageSummary     `json:"storage"`
	Prometheus     PrometheusInfo     `json:"prometheus"`
	Compliance     ComplianceInfo     `json:"compliance"`
	BestPractices  BestPractices      `json:"bestPractices"`
	PVCs           PVCSummary         `json:"pvcs"`
	CoverageMatrix          []PolicyCoverageRow         `json:"policyCoverageMatrix"`
	K10Reports              []K10Report                 `json:"k10Reports"`
	RecentFailures          []RecentFailure             `json:"recentFailures,omitempty"`
	LongRunningActions      []LongRunningAction         `json:"longRunningActions,omitempty"`
	BackupRecency           []NamespaceBackupRecency    `json:"backupRecency,omitempty"`
	StorageClasses          []StorageClassInfo          `json:"storageClasses,omitempty"`
	VolumeSnapshotClasses   []VolumeSnapshotClassInfo   `json:"volumeSnapshotClasses,omitempty"`
	CSIWarnings             []string                    `json:"csiWarnings,omitempty"`
	// QBR analytics
	RecoveryReadiness  RecoveryReadinessScore `json:"recoveryReadiness"`
	AppRiskMatrix      []AppRisk              `json:"appRiskMatrix,omitempty"`
	WeeklySLATrend     []WeeklySLA            `json:"weeklySLATrend,omitempty"`
}

// ── Recovery Readiness Score ──────────────────────────────────────────────────

type RecoveryReadinessScore struct {
	Score         int            `json:"score"`
	Grade         string         `json:"grade"`
	Components    map[string]int `json:"components"`
	MaxComponents map[string]int `json:"maxComponents"`
	Findings      []string       `json:"findings"`
}

// ── App Risk Matrix ───────────────────────────────────────────────────────────

type AppRisk struct {
	Namespace    string   `json:"namespace"`
	Protected    bool     `json:"protected"`
	HasExport    bool     `json:"hasExport"`
	HasImmutable bool     `json:"hasImmutable"`
	RPOHours     float64  `json:"rpoHours,omitempty"`
	RTOMinutes   float64  `json:"rtoMinutes,omitempty"`
	LastBackup   string   `json:"lastBackup,omitempty"`
	RiskLevel    string   `json:"riskLevel"`
	RiskReasons  []string `json:"riskReasons,omitempty"`
}

// ── Weekly SLA Trend ──────────────────────────────────────────────────────────

type WeeklySLA struct {
	Week        string  `json:"week"`
	Label       string  `json:"label"`
	Complete    int     `json:"complete"`
	Failed      int     `json:"failed"`
	Skipped     int     `json:"skipped"`
	SuccessRate float64 `json:"successRate"`
}

// ── Helm / Config ──────────────────────────────────────────────────────────────

type HelmConfig struct {
	ReleaseName          string            `json:"releaseName"`
	ChartVersion         string            `json:"chartVersion"`
	Namespace            string            `json:"namespace"`
	Values               map[string]string `json:"keyValues"`
	ConcurrencyLimit     int               `json:"concurrencyLimit"`
	BackupTimeout        string            `json:"backupTimeout"`
	RestoreTimeout       string            `json:"restoreTimeout"`
	DatastoreParallelism int               `json:"datastoreParallelism"`
	FIPSMode             bool              `json:"fipsMode"`
	AuditLogging         bool              `json:"auditLogging"`
	NetworkPolicies      bool              `json:"networkPoliciesEnabled"`
	DashboardAccess      string            `json:"dashboardAccess"`
	IngressTLS           bool              `json:"ingressTLS"`
}

// ── License ────────────────────────────────────────────────────────────────────

type License struct {
	ProductName   string `json:"productName"`
	Company       string `json:"company"`
	ExpiresAt     string `json:"expiresAt"`
	LicenseType   string `json:"licenseType"`
	NodeLimit     int    `json:"nodeLimit"`
	NodeUsage     int    `json:"nodeUsage"`
	Consumption   string `json:"consumption"`
	Valid         bool   `json:"valid"`
	ExpiresInDays int    `json:"expiresInDays"`
}

// ── Multi-cluster ──────────────────────────────────────────────────────────────

type MultiClusterInfo struct {
	Mode       string          `json:"mode"`
	PrimaryURL string          `json:"primaryUrl,omitempty"`
	Clusters   []RemoteCluster `json:"registeredClusters,omitempty"`
}

type RemoteCluster struct {
	Name    string `json:"name"`
	URL     string `json:"url"`
	Status  string `json:"status"`
	Version string `json:"version,omitempty"`
}

// ── Disaster Recovery ─────────────────────────────────────────────────────────

type DRInfo struct {
	Enabled       bool   `json:"enabled"`
	Mode          string `json:"mode,omitempty"`
	BackupPolicy  string `json:"backupPolicy,omitempty"`
	RestorePoint  string `json:"latestRestorePoint,omitempty"`
	ExportProfile string `json:"exportProfile,omitempty"`
	LastRunTime   string `json:"lastRunTime,omitempty"`
	LastRunStatus string `json:"lastRunStatus,omitempty"`
}

// ── Security ──────────────────────────────────────────────────────────────────

type SecurityConfig struct {
	AuthMethod string           `json:"authMethod"`
	OIDCConfig *OIDCInfo        `json:"oidc,omitempty"`
	LDAPConfig *LDAPInfo        `json:"ldap,omitempty"`
	Encryption EncryptionConfig `json:"encryption"`
}

type OIDCInfo struct {
	ProviderURL   string `json:"providerUrl"`
	ClientID      string `json:"clientId"`
	UsernameClaim string `json:"usernameClaim,omitempty"`
	GroupsClaim   string `json:"groupsClaim,omitempty"`
}

type LDAPInfo struct {
	Host        string `json:"host"`
	BindDN      string `json:"bindDn,omitempty"`
	UserSearch  string `json:"userSearch,omitempty"`
	GroupSearch string `json:"groupSearch,omitempty"`
}

type EncryptionConfig struct {
	Enabled  bool   `json:"enabled"`
	Provider string `json:"provider,omitempty"`
	KeyID    string `json:"keyId,omitempty"`
	VaultURL string `json:"vaultUrl,omitempty"`
}

// ── Applications ──────────────────────────────────────────────────────────────

type AppSummary struct {
	Total        int       `json:"total"`
	Protected    int       `json:"protected"`
	Unprotected  int       `json:"unprotected"`
	Compliant    int       `json:"compliant"`
	NonCompliant int       `json:"nonCompliant"`
	Unmanaged    int       `json:"unmanaged"`
	Excluded     int       `json:"excluded"`
	Apps         []AppInfo `json:"details"`
}

type AppInfo struct {
	Name        string   `json:"name"`
	Namespace   string   `json:"namespace"`
	Protected   bool     `json:"protected"`
	PolicyNames []string `json:"policyNames,omitempty"`
	LastBackup  string   `json:"lastBackupTime,omitempty"`
	Compliant   bool     `json:"compliant"`
}

// ── Namespaces ────────────────────────────────────────────────────────────────

type NamespaceSummary struct {
	Total       int             `json:"total"`
	Excluded    []string        `json:"excluded,omitempty"`
	Unprotected []UnprotectedNS `json:"unprotected,omitempty"`
}

type UnprotectedNS struct {
	Name   string            `json:"name"`
	Labels map[string]string `json:"labels,omitempty"`
}

// ── Policies ──────────────────────────────────────────────────────────────────

type Policy struct {
	Name            string        `json:"name"`
	Namespace       string        `json:"namespace"`
	Enabled         bool          `json:"enabled"`
	IsSystemPolicy  bool          `json:"isSystemPolicy"`
	IsClusterScoped bool          `json:"isClusterScoped,omitempty"`
	IsWildcard      bool          `json:"isWildcard,omitempty"`
	Action          string        `json:"action"`
	Frequency       string        `json:"frequency"`
	Retention       RetentionInfo `json:"retention"`
	Selector        string        `json:"selector,omitempty"`
	ExportProfiles  []string      `json:"exportProfiles,omitempty"`
	LastRunTime     string        `json:"lastRunTime,omitempty"`
	LastRunStatus   string        `json:"lastRunStatus,omitempty"`
	LastRunDuration string        `json:"lastRunDuration,omitempty"`
	AvgRunDuration  string        `json:"avgRunDuration,omitempty"`
	CreatedAt       string        `json:"createdAt"`
	SubType         string        `json:"subType,omitempty"`
}

type RetentionInfo struct {
	Hourly  int `json:"hourly,omitempty"`
	Daily   int `json:"daily,omitempty"`
	Weekly  int `json:"weekly,omitempty"`
	Monthly int `json:"monthly,omitempty"`
	Yearly  int `json:"yearly,omitempty"`
}

type PolicyPreset struct {
	Name      string        `json:"name"`
	Namespace string        `json:"namespace"`
	Action    string        `json:"action"`
	Frequency string        `json:"frequency"`
	Retention RetentionInfo `json:"retention"`
	CreatedAt string        `json:"createdAt"`
}

// ── Profiles ──────────────────────────────────────────────────────────────────

type Profile struct {
	Name               string  `json:"name"`
	Namespace          string  `json:"namespace"`
	Type               string  `json:"type"`
	Provider           string  `json:"provider,omitempty"`
	Bucket             string  `json:"bucket,omitempty"`
	Region             string  `json:"region,omitempty"`
	Endpoint           string  `json:"endpoint,omitempty"`
	Immutability       bool    `json:"immutabilityEnabled"`
	ImmutabilityPeriod string  `json:"immutabilityPeriod,omitempty"`
	StorageUsedGB      float64 `json:"storageUsedGB,omitempty"`
	DedupeRatio        float64 `json:"deduplicationRatio,omitempty"`
	Ready              bool    `json:"ready"`
	CreatedAt          string  `json:"createdAt"`
}

// ── Jobs ──────────────────────────────────────────────────────────────────────

type Job struct {
	Name        string `json:"name"`
	Namespace   string `json:"namespace"`
	PolicyName  string `json:"policyName,omitempty"`
	AppName     string `json:"appName,omitempty"`
	Action      string `json:"action"`
	Status      string `json:"status"`
	StartTime   string `json:"startTime,omitempty"`
	EndTime     string `json:"endTime,omitempty"`
	Duration    string `json:"duration,omitempty"`
	DurationSec int64  `json:"durationSeconds,omitempty"`
	Error       string `json:"error,omitempty"`
}

// JobSummary holds aggregated job statistics.
type JobSummary struct {
	Total     int            `json:"total"`
	Completed int            `json:"completed"`
	Failed    int            `json:"failed"`
	Skipped   int            `json:"skipped"`
	Cancelled int            `json:"cancelled"`
	Running   int            `json:"running"`
	ByAction  map[string]int `json:"byAction"`
	ByStatus  map[string]int `json:"byStatus"`
}

// ── Restore Points ────────────────────────────────────────────────────────────

type RestorePointInfo struct {
	Total    int            `json:"total"`
	Orphaned int            `json:"orphaned"`
	ByApp    map[string]int `json:"byApplication"`
	ByPolicy map[string]int `json:"byPolicy,omitempty"`
	Oldest   string         `json:"oldest,omitempty"`
	Newest   string         `json:"newest,omitempty"`
	Details  []RestorePoint `json:"details,omitempty"`
}

type RestorePoint struct {
	Name      string `json:"name"`
	AppName   string `json:"appName"`
	Policy    string `json:"policy,omitempty"`
	CreatedAt string `json:"createdAt"`
	Orphaned  bool   `json:"orphaned"`
}

// ── KubeVirt ──────────────────────────────────────────────────────────────────

type KubeVirtInfo struct {
	Enabled        bool     `json:"enabled"`
	Version        string   `json:"version,omitempty"`
	TotalVMs       int      `json:"totalVMs"`
	ProtectedVMs   int      `json:"protectedVMs"`
	UnprotectedVMs int      `json:"unprotectedVMs"`
	VMs            []VMInfo `json:"vms,omitempty"`
}

type VMInfo struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Status    string `json:"status"`
	Protected bool   `json:"protected"`
	Policy    string `json:"policy,omitempty"`
}

// ── Kanister ──────────────────────────────────────────────────────────────────

type Blueprint struct {
	Name      string   `json:"name"`
	Namespace string   `json:"namespace"`
	Actions   []string `json:"actions"`
	CreatedAt string   `json:"createdAt"`
}

type BlueprintBinding struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Blueprint string `json:"blueprint"`
	Subject   string `json:"subject"`
	CreatedAt string `json:"createdAt"`
}

type TransformSet struct {
	Name       string `json:"name"`
	Namespace  string `json:"namespace"`
	Transforms int    `json:"transformCount"`
	CreatedAt  string `json:"createdAt"`
}

// ── K10 Resource Limits ───────────────────────────────────────────────────────

type K10Resources struct {
	Deployments []DeploymentInfo `json:"deployments"`
}

type DeploymentInfo struct {
	Name       string         `json:"name"`
	Replicas   int32          `json:"replicas"`
	Ready      int32          `json:"readyReplicas"`
	Containers []ContainerRes `json:"containers"`
}

type ContainerRes struct {
	Name       string `json:"name"`
	CPURequest string `json:"cpuRequest,omitempty"`
	CPULimit   string `json:"cpuLimit,omitempty"`
	MemRequest string `json:"memoryRequest,omitempty"`
	MemLimit   string `json:"memoryLimit,omitempty"`
}

// ── Catalog ───────────────────────────────────────────────────────────────────

type CatalogInfo struct {
	SizeBytes     int64   `json:"sizeBytes"`
	SizeHuman     string  `json:"sizeHuman"`
	FreeBytes     int64   `json:"freeBytes,omitempty"`
	FreeHuman     string  `json:"freeHuman,omitempty"`
	FreePercent   float64 `json:"freePercent,omitempty"`
	LowSpaceAlert bool    `json:"lowSpaceAlert"`
	StorageClass  string  `json:"storageClass,omitempty"`
}

// ── Storage Summary (from K10 reports CRD) ────────────────────────────────────

type StorageSummary struct {
	// Source metadata — when the K10 report was generated
	ReportDate    string `json:"reportDate,omitempty"`    // ISO timestamp of the K10 report used
	ReportAgeDays int    `json:"reportAgeDays,omitempty"` // days since report was generated (-1 = no report)
	// Snapshot storage — local snapshots on cluster
	SnapshotSizeBytes  int64   `json:"snapshotSizeBytes"`
	SnapshotSizeHuman  string  `json:"snapshotSizeHuman"`
	SnapshotCount      int     `json:"snapshotCount"`
	// Export storage — data sent to location profiles
	ExportSizeBytes    int64   `json:"exportSizeBytes"`
	ExportSizeHuman    string  `json:"exportSizeHuman"`
	DedupeRatio        float64 `json:"deduplicationRatio"`
	// Export by application
	ExportByApp        map[string]int64 `json:"exportByApplication"`
	// Live storage — total cluster PVC capacity
	LiveSizeBytes      int64   `json:"liveSizeBytes"`
	LiveSizeHuman      string  `json:"liveSizeHuman"`
	LiveVolumeCount    int     `json:"liveVolumeCount"`
	// K10 services disk usage
	ServicesDisk       []ServiceDiskUsage `json:"servicesDiskUsage"`
}

type ServiceDiskUsage struct {
	Name        string  `json:"name"`
	UsedBytes   int64   `json:"usedBytes"`
	FreeBytes   int64   `json:"freeBytes"`
	TotalBytes  int64   `json:"totalBytes"`
	UsedHuman   string  `json:"usedHuman"`
	FreeHuman   string  `json:"freeHuman"`
	TotalHuman  string  `json:"totalHuman"`
	FreePercent float64 `json:"freePercent"`
}

// ── K10 Reports CRD ───────────────────────────────────────────────────────────

type K10Report struct {
	Name             string             `json:"name"`
	GeneratedAt      string             `json:"generatedAt"`
	Period           string             `json:"period"`
	K10Version       string             `json:"k10Version,omitempty"`
	Stats            K10ReportStats     `json:"stats"`
	ServicesDisk     []ServiceDiskUsage `json:"servicesDisk,omitempty"`
	ProfileSummaries []K10ReportProfile `json:"profileSummaries,omitempty"`
}

type K10ReportStats struct {
	Apps      K10ReportApps     `json:"applications"`
	Actions   K10ReportActions  `json:"actions"`
	Storage   K10ReportStorage  `json:"storage"`
	License   K10ReportLicense  `json:"license"`
	PVCBytes  int64             `json:"pvcBytes"`
	PVCCount  int               `json:"pvcCount"`
}

type K10ReportApps struct {
	Total        int `json:"total"`
	Compliant    int `json:"compliant"`
	NonCompliant int `json:"nonCompliant"`
	Unmanaged    int `json:"unmanaged"`
}

type K10ReportActions struct {
	Total     int `json:"total"`
	Completed int `json:"completed"`
	Failed    int `json:"failed"`
	Skipped   int `json:"skipped"`
	Cancelled int `json:"cancelled"`
	Snapshot  int `json:"snapshot"`
	Restore   int `json:"restore"`
	Export    int `json:"export"`
	Import    int `json:"import"`
}

type K10ReportStorage struct {
	SnapshotSizeBytes int64   `json:"snapshotSizeBytes"`
	SnapshotCount     int     `json:"snapshotCount"`
	ExportSizeBytes   int64   `json:"exportSizeBytes"`
	ExportCount       int     `json:"exportCount"`
	DedupeRatio       float64 `json:"deduplicationRatio"`
}

// K10ReportProfile holds enriched profile data from the report CRD.
type K10ReportProfile struct {
	Name             string `json:"name"`
	Type             string `json:"type"`
	Provider         string `json:"provider"`
	Bucket           string `json:"bucket"`
	Region           string `json:"region"`
	Endpoint         string `json:"endpoint"`
	SSLVerification  string `json:"sslVerification"`
	Validation       string `json:"validation"`
	Immutability     bool   `json:"immutability"`
	ImmutabilityDays int    `json:"immutabilityDays"`
}

type K10ReportLicense struct {
	Type       string `json:"type"`
	Status     string `json:"status"`
	ExpiresAt  string `json:"expiresAt"`
	NodeCount  int    `json:"nodeCount"`
	NodeLimit  int    `json:"nodeLimit"`
}

// ── Prometheus ────────────────────────────────────────────────────────────────

type PrometheusInfo struct {
	Enabled          bool   `json:"enabled"`
	ServiceMonitor   bool   `json:"serviceMonitorCreated"`
	GrafanaDashboard bool   `json:"grafanaDashboardCreated"`
	AlertRules       bool   `json:"alertRulesConfigured"`
	Endpoint         string `json:"endpoint,omitempty"`
}

// ── Compliance ────────────────────────────────────────────────────────────────

type ComplianceInfo struct {
	ProtectionCoverage float64           `json:"protectionCoveragePercent"`
	PolicyCompliance   float64           `json:"policyCompliancePercent"`
	FailedJobs24h      int               `json:"failedJobsLast24h"`
	FailedJobs7d       int               `json:"failedJobsLast7d"`
	SuccessRate7d      float64           `json:"successRateLast7dPercent"`
	UnprotectedApps    []string          `json:"unprotectedApplications,omitempty"`
	PolicyStatus       map[string]string `json:"policyStatus,omitempty"`
	AvgPolicyDuration  map[string]string `json:"avgPolicyDuration,omitempty"`
}

// ── Best Practices ────────────────────────────────────────────────────────────

type BestPractices struct {
	TotalChecks int       `json:"totalChecks"`
	Passed      int       `json:"passed"`
	Warnings    int       `json:"warnings"`
	Critical    int       `json:"critical"`
	Checks      []BPCheck `json:"checks"`
}

type BPCheck struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Status   string `json:"status"`
	Severity string `json:"severity"`
	Detail   string `json:"detail"`
}

// ── PVCs ──────────────────────────────────────────────────────────────────────

type PVCSummary struct {
	Total       int       `json:"total"`
	Bound       int       `json:"bound"`
	Pending     int       `json:"pending"`
	Lost        int       `json:"lost"`
	TotalSizeGB float64   `json:"totalSizeGB"`
	Items       []PVCInfo `json:"items"`
}

type PVCInfo struct {
	Name         string  `json:"name"`
	Namespace    string  `json:"namespace"`
	Status       string  `json:"status"`
	CapacityGB   float64 `json:"capacityGB"`
	StorageClass string  `json:"storageClass"`
	AccessModes  string  `json:"accessModes"`
}

// ── Policy Coverage Matrix ────────────────────────────────────────────────────

type PolicyCoverageRow struct {
	Namespace  string   `json:"namespace"`
	Protected  bool     `json:"protected"`
	Policies   []string `json:"policies"`
	Frequency  string   `json:"frequency,omitempty"`
	LastBackup string   `json:"lastBackup,omitempty"`
}

// ── Failed Actions Top-N ──────────────────────────────────────────────────────

type RecentFailure struct {
	Kind       string `json:"kind"`
	Name       string `json:"name"`
	AppName    string `json:"appName,omitempty"`
	PolicyName string `json:"policyName,omitempty"`
	StartTime  string `json:"startTime,omitempty"`
	Error      string `json:"error,omitempty"`
}

// ── Stuck Actions ────────────────────────────────────────────────────────────

type LongRunningAction struct {
	Kind       string `json:"kind"`
	Name       string `json:"name"`
	AppName    string `json:"appName,omitempty"`
	PolicyName string `json:"policyName,omitempty"`
	StartTime  string `json:"startTime,omitempty"`
	RunningFor string `json:"runningFor"`
}

// ── Namespace Protection Status ───────────────────────────────────────────────

type NamespaceBackupRecency struct {
	Namespace      string `json:"namespace"`
	Protected      bool   `json:"protected"`
	LastBackup     string `json:"lastBackupTime,omitempty"`
	LastExport     string `json:"lastExportTime,omitempty"`
	LastRestore    string `json:"lastRestoreTime,omitempty"`
	DaysSinceBackup int   `json:"daysSinceLastBackup,omitempty"`
	Drift          bool   `json:"backupDrift"`
}

// ── StorageClass / VolumeSnapshotClass inventory ─────────────────────────────

type StorageClassInfo struct {
	Name          string `json:"name"`
	Provisioner   string `json:"provisioner"`
	IsDefault     bool   `json:"isDefault"`
	Expandable    bool   `json:"allowVolumeExpansion"`
	ReclaimPolicy string `json:"reclaimPolicy"`
	BindingMode   string `json:"bindingMode"`
	HasVSC        bool   `json:"hasMatchingVolumeSnapshotClass"`
}

type VolumeSnapshotClassInfo struct {
	Name                string `json:"name"`
	Driver              string `json:"driver"`
	IsDefault           bool   `json:"isDefault"`
	DeletionPolicy      string `json:"deletionPolicy"`
	HasKastenAnnotation bool   `json:"hasKastenAnnotation"`
}
