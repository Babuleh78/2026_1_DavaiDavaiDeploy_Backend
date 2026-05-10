package models

import "errors"

type SaveRatingInput struct {
    VideoID       string `json:"video_id"`
    Physical      int    `json:"physical"`
    Speed         int    `json:"speed"`
    Coordination  int    `json:"coordination"`
    Repeatability int    `json:"repeatability"`
}

type RatingResponse struct {
    VideoID          string  `json:"video_id"`
    AvgPhysical      float64 `json:"avg_physical"`
    AvgSpeed         float64 `json:"avg_speed"`
    AvgCoordination  float64 `json:"avg_coordination"`
    AvgRepeatability float64 `json:"avg_repeatability"`
    AvgScore         float64 `json:"avg_score"`
    TotalRatings     int     `json:"total_ratings"`
}

func (s *SaveRatingInput) Validate() error {
    for _, v := range []int{s.Physical, s.Speed, s.Coordination, s.Repeatability} {
        if v < 1 || v > 10 {
            return errors.New("rating values must be between 1 and 10")
        }
    }
    return nil
}