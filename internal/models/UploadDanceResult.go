package models
type UploadDanceResult struct {
    DanceID             string
    SegmentsKey         string
	VideoPath			string
	FullGlbKey          string
    GlbKeys             []string
    NumFrames           int
    NumSegments         int
    NumSegmentsRendered int
    DurationSec         float64
    LikesCount          int64
    IsLiked             bool

}
