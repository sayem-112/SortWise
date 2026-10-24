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
	// AvatarURL is where X serves the author's profile picture, when X's own
	// data said. It is never accepted from JSON input.
	AvatarURL string `json:"-"`
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
	// AuthorAvatar is the path of the author's saved profile picture, if any.
	AuthorAvatar string         `json:"authorAvatar,omitempty"`
	Categories   []TaxonomyItem `json:"categories"`
	Tags         []TaxonomyItem `json:"tags"`
	// Lists holds the IDs of the lists the bookmark is in.
	Lists []int64 `json:"lists"`
	// Match is "meaning" when a search found the post by what it is about,
	// with none of the words searched for in it.
	Match string `json:"match,omitempty"`
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
	// Search is set when a search by meaning was asked for.
	Search *SearchInfo `json:"search,omitempty"`
}

// SearchInfo says whether posts close in meaning were included in a search,
// and, when they could not be, why.
type SearchInfo struct {
	Meaning bool   `json:"meaning"`
	Notice  string `json:"notice,omitempty"`
}

type BookmarkQuery struct {
	Text        string
	CategoryIDs []int64
	TagIDs      []int64
	// ListID limits results to one list; they are then sorted by when they
	// were added to it unless another order is asked for.
	ListID int64
	// NoList limits results to bookmarks that are in no list at all.
	NoList bool
	// Author limits results to one X handle, whatever its capitalization.
	Author          string
	ProcessingState string
	Sort            string
	IncludeArchived bool
	Page            int
	PageSize        int
	// Hybrid adds the posts in Semantic (closest in meaning first) to the
	// posts that contain the words in Text, and ranks the two together.
	Hybrid   bool
	Semantic []int64
}

// CollectionQuery is the search and filters a smart collection saves, in the
// same terms the library's own controls use.
type CollectionQuery struct {
	Text string `json:"q"`
	// Mode is "meaning" to also find posts about the same thing, or "".
	Mode       string  `json:"mode"`
	Categories []int64 `json:"categories"`
	Tags       []int64 `json:"tags"`
	Status     string  `json:"status"`
	// List is a list's ID, "none" for posts in no list, or "".
	List   string `json:"list"`
	Author string `json:"author"`
	Sort   string `json:"sort"`
}

// Collection is a saved search that shows whatever matches it now.
type Collection struct {
	ID    int64           `json:"id"`
	Name  string          `json:"name"`
	Color string          `json:"color"`
	Query CollectionQuery `json:"query"`
	Count int             `json:"count"`
}

// CollectionUpdate changes a collection's name, color, or saved search; nil
// fields stay as they are.
type CollectionUpdate struct {
	Name  *string          `json:"name"`
	Color *string          `json:"color"`
	Query *CollectionQuery `json:"query"`
}

// RelatedBookmark is a post close in meaning to another one.
type RelatedBookmark struct {
	ID       int64   `json:"id"`
	Author   string  `json:"author"`
	Username string  `json:"username"`
	Avatar   string  `json:"avatar,omitempty"`
	PostedAt *string `json:"postedAt"`
	// Text is the best one-line description of the post.
	Text string `json:"text"`
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
	Icon  string `json:"icon"`
	Color string `json:"color"`
	// Pinned lists sort first, after Favorites.
	Pinned bool `json:"pinned"`
	Count  int  `json:"count"`
}

// ListUpdate changes a list's name, icon, color, or pin; empty fields stay as
// they are.
type ListUpdate struct {
	Name   string `json:"name"`
	Icon   string `json:"icon"`
	Color  string `json:"color"`
	Pinned *bool  `json:"pinned"`
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

// Author is someone whose posts are saved, keyed by X handle.
type Author struct {
	Username      string `json:"username"`
	Name          string `json:"name"`
	Count         int    `json:"count"`
	LatestSavedAt string `json:"latestSavedAt"`
	// Avatar is the path of their saved profile picture, if any.
	Avatar string `json:"avatar,omitempty"`
}

type AuthorQuery struct {
	Text     string
	Sort     string
	Page     int
	PageSize int
}

type AuthorPage struct {
	Items    []Author `json:"items"`
	Page     int      `json:"page"`
	PageSize int      `json:"pageSize"`
	Total    int      `json:"total"`
}

// AuthorProfile is what the top of an author's page shows.
type AuthorProfile struct {
	Username      string `json:"username"`
	Name          string `json:"name"`
	Count         int    `json:"count"`
	LatestSavedAt string `json:"latestSavedAt"`
	Avatar        string `json:"avatar,omitempty"`
	// When their saved posts were written; empty if no post has a date.
	FirstPostedAt  string       `json:"firstPostedAt"`
	LatestPostedAt string       `json:"latestPostedAt"`
	Categories     []NamedCount `json:"categories"`
	TopTags        []NamedCount `json:"topTags"`
}

// QuickBookmark is one post in the finder's results. Text is an excerpt of
// the post with any matched words marked. The other excerpts are empty unless
// the match is in them: the summary, what is in the images, the tags and
// categories, or the author's name or handle.
type QuickBookmark struct {
	ID            int64   `json:"id"`
	Author        string  `json:"author"`
	Username      string  `json:"username"`
	Avatar        string  `json:"avatar,omitempty"`
	PostedAt      *string `json:"postedAt"`
	ImportedAt    string  `json:"importedAt"`
	Text          string  `json:"text"`
	Summary       string  `json:"summary,omitempty"`
	Media         string  `json:"media,omitempty"`
	Taxonomy      string  `json:"taxonomy,omitempty"`
	AuthorMatch   string  `json:"authorMatch,omitempty"`
	UsernameMatch string  `json:"usernameMatch,omitempty"`
}

type QuickResults struct {
	Bookmarks []QuickBookmark `json:"bookmarks"`
	Total     int             `json:"total"`
	Authors   []Author        `json:"authors"`
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
