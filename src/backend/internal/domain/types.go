package domain

import "time"

type Profile struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Host      string    `json:"host"`
	Port      int       `json:"port"`
	Username  string    `json:"username"`
	Password  string    `json:"password,omitempty"`
	BasePath  string    `json:"base_path"`
	Preset    string    `json:"preset"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type LibraryRoot struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	Path     string `json:"-"`
	Favorite bool   `json:"favorite"`
	Kind     string `json:"kind"`
}

type Entry struct {
	Name       string    `json:"name"`
	Path       string    `json:"path"`
	IsDir      bool      `json:"is_dir"`
	Size       int64     `json:"size"`
	ModifiedAt time.Time `json:"modified_at,omitempty"`
	GameKind   string    `json:"game_kind,omitempty"`
}

type SourceLocator struct {
	RootID string `json:"root_id"`
	Path   string `json:"path"`
}

type ExtractionTask struct {
	ID                string        `json:"id"`
	Source            SourceLocator `json:"source"`
	DestinationParent SourceLocator `json:"destination_parent"`
	Destination       SourceLocator `json:"destination"`
	DeleteSources     bool          `json:"delete_sources"`
	Password          string        `json:"-"`
	State             string        `json:"state"`
	TotalBytes        int64         `json:"total_bytes"`
	ExtractedBytes    int64         `json:"extracted_bytes"`
	SpeedBytes        float64       `json:"speed_bytes"`
	ETASeconds        *int64        `json:"eta_seconds"`
	CurrentFile       string        `json:"current_file,omitempty"`
	TotalItems        int           `json:"total_items"`
	CompletedItems    int           `json:"completed_items"`
	Error             string        `json:"error,omitempty"`
	Warning           string        `json:"warning,omitempty"`
	RetryOf           string        `json:"retry_of,omitempty"`
	CreatedAt         time.Time     `json:"created_at"`
	StartedAt         *time.Time    `json:"started_at,omitempty"`
	FinishedAt        *time.Time    `json:"finished_at,omitempty"`
}

type Task struct {
	ID               string          `json:"id"`
	Type             string          `json:"type"`
	ProfileID        string          `json:"profile_id"`
	Sources          []SourceLocator `json:"sources"`
	Destination      string          `json:"destination"`
	ConflictPolicy   string          `json:"conflict_policy"`
	State            string          `json:"state"`
	TotalBytes       int64           `json:"total_bytes"`
	TransferredBytes int64           `json:"transferred_bytes"`
	SpeedBytes       float64         `json:"speed_bytes"`
	ETASeconds       *int64          `json:"eta_seconds"`
	CurrentFile      string          `json:"current_file,omitempty"`
	TotalItems       int             `json:"total_items"`
	CompletedItems   int             `json:"completed_items"`
	SkippedItems     int             `json:"skipped_items"`
	Error            string          `json:"error,omitempty"`
	RetryOf          string          `json:"retry_of,omitempty"`
	CreatedAt        time.Time       `json:"created_at"`
	StartedAt        *time.Time      `json:"started_at,omitempty"`
	FinishedAt       *time.Time      `json:"finished_at,omitempty"`
}

type TaskItem struct {
	ID          string `json:"id"`
	TaskID      string `json:"task_id"`
	RootID      string `json:"root_id"`
	SourcePath  string `json:"source_path"`
	Destination string `json:"destination"`
	IsDir       bool   `json:"is_dir"`
	Size        int64  `json:"size"`
	ModUnixNano int64  `json:"-"`
	State       string `json:"state"`
	Transferred int64  `json:"transferred"`
	Attempts    int    `json:"attempts"`
	Error       string `json:"error,omitempty"`
}

const (
	TaskQueued      = "queued"
	TaskScanning    = "scanning"
	TaskRunning     = "running"
	TaskCanceling   = "canceling"
	TaskSucceeded   = "succeeded"
	TaskFailed      = "failed"
	TaskCanceled    = "canceled"
	TaskInterrupted = "interrupted"
)

const (
	ExtractionQueued      = "queued"
	ExtractionScanning    = "scanning"
	ExtractionExtracting  = "extracting"
	ExtractionCleaning    = "cleaning"
	ExtractionCanceling   = "canceling"
	ExtractionSucceeded   = "succeeded"
	ExtractionFailed      = "failed"
	ExtractionCanceled    = "canceled"
	ExtractionInterrupted = "interrupted"
)

func ExtractionTerminal(state string) bool {
	return state == ExtractionSucceeded || state == ExtractionFailed || state == ExtractionCanceled || state == ExtractionInterrupted
}

func ValidConflictPolicy(v string) bool {
	return v == "smart" || v == "overwrite" || v == "fail"
}
