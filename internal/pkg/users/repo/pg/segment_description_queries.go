package repo

import _ "embed"

//go:embed sql/segment_descriptions/upsert_segment_description.sql
var UpsertSegmentDescriptionQuery string

//go:embed sql/segment_descriptions/get_segment_description.sql
var GetSegmentDescriptionQuery string

//go:embed sql/segment_descriptions/get_dance_segment_descriptions.sql
var GetDanceSegmentDescriptionsQuery string
