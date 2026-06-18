package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"DDDance/internal/models"
)

const mlInternalTokenHeader = "X-Internal-Token"

type MLClient struct {
	baseURL    string
	httpClient *http.Client
}

func NewMLClient(baseURL string) *MLClient {
	return &MLClient{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *MLClient) mlURL(path string) string {
	return strings.TrimRight(c.baseURL, "/") + "/ml/" + strings.TrimLeft(path, "/")
}

func (c *MLClient) setAuthHeader(req *http.Request) {
	if token := os.Getenv("ML_INTERNAL_TOKEN"); token != "" {
		req.Header.Set(mlInternalTokenHeader, token)
	}
}

func (c *MLClient) Recommend(ctx context.Context, query string, dances []models.RecommenderDanceItem, limit int) (recommendedIDs []string, reasoning string, err error) {
	dancesPayload := make([]map[string]interface{}, 0, len(dances))
	for _, d := range dances {
		dancesPayload = append(dancesPayload, map[string]interface{}{
			"id":          d.ID,
			"title":       d.Title,
			"description": d.Description,
			"avg_score":   d.AvgScore,
			"view_count":  d.ViewCount,
		})
	}

	body, _ := json.Marshal(map[string]interface{}{
		"query":  query,
		"dances": dancesPayload,
		"limit":  limit,
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.mlURL("recommend"), bytes.NewBuffer(body))
	if err != nil {
		return nil, "", fmt.Errorf("build recommend request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	c.setAuthHeader(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("recommend request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("recommend endpoint returned %d", resp.StatusCode)
	}

	var mlResp struct {
		RecommendedIDs []string `json:"recommended_ids"`
		Reasoning      string   `json:"reasoning"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&mlResp); err != nil {
		return nil, "", fmt.Errorf("decode recommend response: %w", err)
	}

	return mlResp.RecommendedIDs, mlResp.Reasoning, nil
}

func (c *MLClient) GetSimilar(ctx context.Context, danceID string, dances []models.RecommenderDanceItem, limit int) ([]string, error) {
	dancesPayload := make([]map[string]interface{}, 0, len(dances))
	for _, d := range dances {
		dancesPayload = append(dancesPayload, map[string]interface{}{
			"id":          d.ID,
			"title":       d.Title,
			"description": d.Description,
			"avg_score":   d.AvgScore,
			"view_count":  d.ViewCount,
		})
	}

	body, _ := json.Marshal(map[string]interface{}{
		"dance_id": danceID,
		"dances":   dancesPayload,
		"limit":    limit,
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.mlURL("similar"), bytes.NewBuffer(body))
	if err != nil {
		return nil, fmt.Errorf("build similar request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	c.setAuthHeader(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("similar request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("similar endpoint returned %d", resp.StatusCode)
	}

	var mlResp struct {
		RecommendedIDs []string `json:"recommended_ids"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&mlResp); err != nil {
		return nil, fmt.Errorf("decode similar response: %w", err)
	}

	return mlResp.RecommendedIDs, nil
}

func (c *MLClient) GetPersonalizedReels(ctx context.Context, history []models.UserReelsHistoryItem, candidates []models.RecommenderDanceItem, limit int, excludeIDs []string, behaviorLog []models.BehaviorLogEntry, friendUploaderIDs []string) ([]string, error) {
	histPayload := make([]map[string]interface{}, 0, len(history))
	for _, h := range history {
		histPayload = append(histPayload, map[string]interface{}{
			"dance_id":  h.DanceID,
			"score":     h.Score,
			"viewed_at": h.ViewedAt,
			"liked":     h.Liked,
		})
	}
	candPayload := make([]map[string]interface{}, 0, len(candidates))
	for _, c := range candidates {
		candPayload = append(candPayload, map[string]interface{}{
			"id":          c.ID,
			"title":       c.Title,
			"description": c.Description,
			"avg_score":   c.AvgScore,
			"view_count":  c.ViewCount,
			"uploader_id": c.UploaderID,
			"created_at":  c.CreatedAt,
		})
	}
	behaviorPayload := make([]map[string]interface{}, 0, len(behaviorLog))
	for _, b := range behaviorLog {
		behaviorPayload = append(behaviorPayload, map[string]interface{}{
			"dance_id":  b.DanceID,
			"action":    b.Action,
			"timestamp": b.Timestamp,
		})
	}

	if friendUploaderIDs == nil {
		friendUploaderIDs = []string{}
	}
	body, _ := json.Marshal(map[string]interface{}{
		"user_history":        histPayload,
		"candidate_dances":    candPayload,
		"limit":               limit,
		"exclude_ids":         excludeIDs,
		"behavior_log":        behaviorPayload,
		"friend_uploader_ids": friendUploaderIDs,
	})

	reelsCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()

	req, reqErr := http.NewRequestWithContext(reelsCtx, http.MethodPost, c.mlURL("reels_feed"), bytes.NewBuffer(body))
	if reqErr != nil {
		return nil, fmt.Errorf("build ml request: %w", reqErr)
	}
	req.Header.Set("Content-Type", "application/json")
	c.setAuthHeader(req)

	plainClient := &http.Client{}
	resp, err := plainClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ml request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ml returned status %d", resp.StatusCode)
	}

	var mlResp struct {
		RecommendedIDs []string `json:"recommended_ids"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&mlResp); err != nil {
		return nil, fmt.Errorf("decode ml response: %w", err)
	}

	if len(mlResp.RecommendedIDs) == 0 {
		return nil, fmt.Errorf("ml returned empty recommendations")
	}

	return mlResp.RecommendedIDs, nil
}
