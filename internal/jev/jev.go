// Package jev implements the TypeSafe HTTP decision API.
package jev

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"time"
)

type Question struct {
	Type         string            `json:"type" yaml:"type"`
	Instructions string            `json:"instructions" yaml:"instructions"`
	Criteria     map[string]string `json:"criteria" yaml:"criteria"`
}
type Request struct {
	Model     string              `json:"model"`
	State     any                 `json:"state"`
	Questions map[string]Question `json:"questions"`
}
type Answer struct {
	Type          string             `json:"type" yaml:"type"`
	Choice        string             `json:"choice" yaml:"choice"`
	Probabilities map[string]float64 `json:"probabilities" yaml:"probabilities"`
	Confidence    float64            `json:"confidence" yaml:"confidence"`
}
type Response struct {
	Model   string            `json:"model" yaml:"model"`
	Answers map[string]Answer `json:"answers" yaml:"answers"`
	Usage   map[string]int    `json:"usage" yaml:"usage"`
}

func Build(state any, model string, questions map[string]Question) Request {
	return Request{Model: model, State: state, Questions: questions}
}

func Evaluate(ctx context.Context, endpoint, key string, req Request) (Response, error) {
	var result Response
	body, err := json.Marshal(req)
	if err != nil {
		return result, err
	}
	// Conservative byte guard; API token limits remain authoritative.
	if len(body) > 100000 {
		return result, fmt.Errorf("request exceeds POC size guard (100000 bytes); reduce module scope")
	}
	request, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(body))
	if err != nil {
		return result, err
	}
	request.Header.Set("Authorization", "Bearer "+key)
	request.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 90 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		return result, err
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return result, fmt.Errorf("TypeSafe returned HTTP %d", response.StatusCode)
	}
	if err = json.NewDecoder(io.LimitReader(response.Body, 4<<20)).Decode(&result); err != nil {
		return result, err
	}
	if result.Model == "" || len(result.Answers) != len(req.Questions) {
		return result, fmt.Errorf("invalid response model or answer count")
	}
	for id, q := range req.Questions {
		a, ok := result.Answers[id]
		if !ok || a.Type != "choice" {
			return result, fmt.Errorf("invalid answer for %s", id)
		}
		if _, ok = q.Criteria[a.Choice]; !ok {
			return result, fmt.Errorf("unknown choice for %s", id)
		}
		if a.Confidence < 0 || a.Confidence > 1 {
			return result, fmt.Errorf("invalid confidence for %s", id)
		}
		if len(a.Probabilities) != len(q.Criteria) {
			return result, fmt.Errorf("invalid probability count for %s", id)
		}
		sum := 0.0
		for option := range q.Criteria {
			p, ok := a.Probabilities[option]
			if !ok || math.IsNaN(p) || p < 0 || p > 1 {
				return result, fmt.Errorf("invalid probability for %s", id)
			}
			sum += p
		}
		if math.Abs(sum-1) > 0.02 {
			return result, fmt.Errorf("probabilities do not sum to one for %s", id)
		}
	}
	return result, nil
}
