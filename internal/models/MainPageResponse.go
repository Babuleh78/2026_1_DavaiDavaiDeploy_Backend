package models

type VideoItem struct {
    ID  string `json:"id"`
    URL string `json:"url"`
}

type MainPageResponse struct {
    Count  int         `json:"count"`
    Videos []VideoItem `json:"videos"`
}