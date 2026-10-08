package model

import "encoding/json"

type VisibleContext struct {
	QuotedPost *QuotedPost `json:"quotedPost,omitempty"`
	Card       *LinkCard   `json:"card,omitempty"`
	// Article marks an X Article whose title, preview, and cover were filled in
	// from X's embed data; EmbedChecked records that the lookup already happened.
	Article      bool `json:"article,omitempty"`
	EmbedChecked bool `json:"embedChecked,omitempty"`
}

type QuotedPost struct {
	PostID   string `json:"postId"`
	Username string `json:"username"`
	URL      string `json:"url"`
	Text     string `json:"text"`
}

type LinkCard struct {
	URL         string `json:"url"`
	Title       string `json:"title"`
	Description string `json:"description"`
}

type ImportedMedia struct {
	Kind       string `json:"kind"`
	URL        string `json:"url"`
	PreviewURL string `json:"previewUrl"`
	AltText    string `json:"altText"`
	Width      *int   `json:"width"`
	Height     *int   `json:"height"`
	// VideoURL is the best MP4 for a video or GIF, when X provided one.
	VideoURL string `json:"videoUrl,omitempty"`
}

type ImportedBookmark struct {
	Platform         string          `json:"platform"`
	PostID           string          `json:"postId"`
	Author           string          `json:"author"`
	Username         string          `json:"username"`
	Text             string          `json:"text"`
	URL              string          `json:"url"`
	PostedAt         *string         `json:"postedAt"`
	BookmarkedAt     *string         `json:"bookmarkedAt"`
	Language         *string         `json:"language"`
	VisibleContext   VisibleContext  `json:"visibleContext"`
	Media            []ImportedMedia `json:"media"`
	ExtractedAt      string          `json:"extractedAt"`
	ExtractorVersion string          `json:"extractorVersion"`
	// Raw is X's original data for the post, kept so a better parser can
	// re-read it later. It is never accepted from JSON input.
	Raw json.RawMessage `json:"-"`
}

type ImportBatch struct {
	Bookmarks []ImportedBookmark `json:"bookmarks"`
	// Source records how the batch arrived: extension (page reading), sync,
	// capture (bookmarked on X just now), or scroll.
	Source string `json:"-"`
}

type ImportResult struct {
	ImportID  int64 `json:"importId"`
	Inserted  int   `json:"inserted"`
	Updated   int   `json:"updated"`
	Unchanged int   `json:"unchanged"`
	Failed    int   `json:"failed"`
	// Upgraded counts saved posts whose data was filled in from X's own data
	// without changing what the AI already analyzed.
	Upgraded int `json:"upgraded"`
	// Skipped counts posts X returned that could not be read (deleted,
	// withheld, or an unknown shape).
	Skipped int `json:"skipped"`
}

type Bookmark struct {
	ID               int64           `json:"id"`
	PostID           string          `json:"postId"`
	Author           string          `json:"author"`
	Username         string          `json:"username"`
	Text             string          `json:"text"`
	URL              string          `json:"url"`
	PostedAt         *string         `json:"postedAt"`
	ImportedAt       string          `json:"importedAt"`
	Language         *string         `json:"language"`
	VisibleContext   VisibleContext  `json:"visibleContext"`
	Media            []ImportedMedia `json:"media"`
	Summary          string          `json:"summary"`
	MediaDescription string          `json:"mediaDescription"`
	ProcessingStatus string          `json:"processingStatus"`
	Archived         bool            `json:"archived"`
	RemovedOnXAt     *string         `json:"removedOnXAt,omitempty"`
	Categories       []TaxonomyItem  `json:"categories"`
	Tags             []TaxonomyItem  `json:"tags"`
	// Lists holds the IDs of the lists the bookmark is in.
	Lists []int64 `json:"lists"`
}

type TaxonomyItem struct {
	ID         int64    `json:"id"`
	Name       string   `json:"name"`
	Confidence *float64 `json:"confidence,omitempty"`
	Manual     bool     `json:"manual"`
}

type BookmarkPage struct {
	Items    []Bookmark `json:"items"`
	Page     int        `json:"page"`
	PageSize int        `json:"pageSize"`
	Total    int        `json:"total"`
}

type BookmarkQuery struct {
	Text        string
	CategoryIDs []int64
	TagIDs      []int64
	// ListID limits results to one list; they are then sorted by when they
	// were added to it unless another order is asked for.
	ListID          int64
	ProcessingState string
	Sort            string
	IncludeArchived bool
	Page            int
	PageSize        int
}

type Category struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	ParentID    *int64 `json:"parentId,omitempty"`
	Description string `json:"description"`
	Active      bool   `json:"active"`
	Count       int    `json:"count"`
}

// List is a list the user keeps bookmarks in by hand. Kind is "favorites"
// for the built-in Favorites list and "custom" otherwise.
type List struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Kind  string `json:"kind"`
	Count int    `json:"count"`
}

type Tag struct {
	ID      int64    `json:"id"`
	Name    string   `json:"name"`
	Kind    string   `json:"kind"`
	Count   int      `json:"count"`
	Aliases []string `json:"aliases"`
}

type AssignmentDecision struct {
	ID    int64  `json:"id"`
	State string `json:"state"`
}

type BookmarkMetadataUpdate struct {
	Summary           *string              `json:"summary,omitempty"`
	ResetSummary      bool                 `json:"resetSummary,omitempty"`
	CategoryDecisions []AssignmentDecision `json:"categoryDecisions,omitempty"`
	TagDecisions      []AssignmentDecision `json:"tagDecisions,omitempty"`
}

type NamedCount struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Count int    `json:"count"`
}
type DashboardStats struct {
	Total     int `json:"total"`
	Completed int `json:"completed"`
	Pending   int `json:"pending"`
	Failed    int `json:"failed"`
	// NeedsReview counts analyzed bookmarks without a confident category.
	NeedsReview int          `json:"needsReview"`
	Categories  []NamedCount `json:"categories"`
	TopTags     []NamedCount `json:"topTags"`
}

func RawJSON(value any) string {
	body, _ := json.Marshal(value)
	return string(body)
}

// TaxonomySnapshot is what the AI sees when proposing structure: the current
// categories and tags with usage counts, plus a sample of analyzed summaries.
type TaxonomySnapshot struct {
	Categories []CategoryStat
	Tags       []TagStat
	Summaries  []string
	Analyzed   int
}

type CategoryStat struct {
	Name        string
	Parent      string
	Description string
	Count       int
}

type TagStat struct {
	Name  string
	Kind  string
	Count int
}

type CategoryProposal struct {
	Name        string   `json:"name"`
	Parent      string   `json:"parent"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
	Reason      string   `json:"reason"`
}

type MergeProposal struct {
	Source string `json:"source"`
	Target string `json:"target"`
	Reason string `json:"reason"`
}

type TaxonomyProposal struct {
	ID       int64             `json:"id"`
	Kind     string            `json:"kind"`
	Category *CategoryProposal `json:"category,omitempty"`
	Merge    *MergeProposal    `json:"merge,omitempty"`
	// Matches is how many bookmarks a category proposal would be applied to, or
	// how many bookmarks the source tag of a merge has.
	Matches int `json:"matches"`
}
