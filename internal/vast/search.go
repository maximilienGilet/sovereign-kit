package vast

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
)

type OfferSort string

const (
	SortPrice       OfferSort = "price"
	SortReliability OfferSort = "reliability"
)

type SearchRequest struct {
	Countries     []string
	Sort          OfferSort
	Limit         int
	GPUModel      string
	GPUCount      int
	StrictGPU     bool
	MinimumVRAMGB int
	MinimumDiskGB int
}

type Offer struct {
	PriceUnknown       bool    `json:"price_unknown,omitempty"`
	ReliabilityUnknown bool    `json:"reliability_unknown,omitempty"`
	ID                 int     `json:"id"`
	MachineID          int     `json:"machine_id"`
	GPUName            string  `json:"gpu_name"`
	GPUCount           int     `json:"num_gpus"`
	GPUVRAMGB          float64 `json:"gpu_ram"`
	TotalGPUVRAMGB     float64 `json:"total_gpu_vram_gb"`
	CPUCores           float64 `json:"cpu_cores_effective"`
	CPURAMGB           float64 `json:"cpu_ram"`
	DiskSpaceGB        float64 `json:"disk_space"`
	InetDownMBps       float64 `json:"inet_down"`
	InetUpMBps         float64 `json:"inet_up"`
	DriverVersion      string  `json:"driver_version"`
	HourlyUSD          float64 `json:"dph_total"`
	Location           string  `json:"geolocation"`
	Reliability        float64 `json:"reliability"`
}

type offerResponse struct {
	ID            int      `json:"id"`
	MachineID     int      `json:"machine_id"`
	GPUName       string   `json:"gpu_name"`
	GPUCount      int      `json:"num_gpus"`
	GPURAMMB      float64  `json:"gpu_ram"`
	CPUCores      float64  `json:"cpu_cores_effective"`
	CPURAMMB      float64  `json:"cpu_ram"`
	DiskSpaceGB   float64  `json:"disk_space"`
	InetDownMBps  float64  `json:"inet_down"`
	InetUpMBps    float64  `json:"inet_up"`
	DriverVersion string   `json:"driver_version"`
	HourlyUSD     *float64 `json:"dph_total"`
	Location      string   `json:"geolocation"`
	Reliability   *float64 `json:"reliability"`
}

func (offer offerResponse) normalized() Offer {
	gpuVRAMGB := offer.GPURAMMB / 1000
	price, priceUnknown := 0.0, true
	if offer.HourlyUSD != nil && *offer.HourlyUSD >= 0 && !math.IsInf(*offer.HourlyUSD, 0) && !math.IsNaN(*offer.HourlyUSD) {
		price = *offer.HourlyUSD
		priceUnknown = false
	}
	reliability, reliabilityUnknown := 0.0, true
	if offer.Reliability != nil && *offer.Reliability >= 0 && *offer.Reliability <= 1 {
		reliability = *offer.Reliability
		reliabilityUnknown = false
	}
	return Offer{
		PriceUnknown:       priceUnknown,
		ReliabilityUnknown: reliabilityUnknown,
		ID:                 offer.ID,
		MachineID:          offer.MachineID,
		GPUName:            offer.GPUName,
		GPUCount:           offer.GPUCount,
		GPUVRAMGB:          gpuVRAMGB,
		TotalGPUVRAMGB:     gpuVRAMGB * float64(offer.GPUCount),
		CPUCores:           offer.CPUCores,
		CPURAMGB:           offer.CPURAMMB / 1000,
		DiskSpaceGB:        offer.DiskSpaceGB,
		InetDownMBps:       offer.InetDownMBps,
		InetUpMBps:         offer.InetUpMBps,
		DriverVersion:      offer.DriverVersion,
		HourlyUSD:          price,
		Location:           offer.Location,
		Reliability:        reliability,
	}
}

// SearchOffers returns on-demand, verified, rentable offers with enough VRAM
// for the selected built-in profile. It never creates an instance.
type searchResponse struct {
	Offers json.RawMessage `json:"offers"`
}

func (client *Client) SearchOffers(ctx context.Context, request SearchRequest) ([]Offer, error) {
	countries, err := NormalizeCountries(request.Countries)
	if err != nil {
		return nil, err
	}
	order := [][]string{{"dph_total", "asc"}}
	switch request.Sort {
	case "", SortPrice:
	case SortReliability:
		order = [][]string{{"reliability", "desc"}, {"dph_total", "asc"}}
	default:
		return nil, fmt.Errorf("invalid offer sort %q", request.Sort)
	}
	if request.Limit < 1 || request.Limit > 100 {
		return nil, fmt.Errorf("offer limit must be between 1 and 100")
	}
	if request.MinimumVRAMGB < 1 {
		return nil, fmt.Errorf("minimum VRAM must be positive")
	}
	if request.MinimumDiskGB < 1 {
		return nil, fmt.Errorf("minimum disk must be positive")
	}
	if client.token == "" {
		return nil, fmt.Errorf("Vast API token is required")
	}
	payload := map[string]any{
		"limit":      request.Limit,
		"type":       "ondemand",
		"verified":   map[string]bool{"eq": true},
		"rentable":   map[string]bool{"eq": true},
		"rented":     map[string]bool{"eq": false},
		"gpu_ram":    map[string]int{"gte": request.MinimumVRAMGB * 1000},
		"disk_space": map[string]int{"gte": request.MinimumDiskGB},
		"order":      order,
	}
	if len(countries) > 0 {
		payload["geolocation"] = map[string][]string{"in": countries}
	}
	if request.StrictGPU {
		payload["gpu_name"] = map[string]string{"eq": request.GPUModel}
		payload["num_gpus"] = map[string]int{"eq": request.GPUCount}
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, client.baseURL+"/api/v0/bundles", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpRequest.Header.Set("Authorization", "Bearer "+client.token)
	httpRequest.Header.Set("Content-Type", "application/json")
	response, err := client.http.Do(httpRequest)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("Vast search offers returned HTTP %d", response.StatusCode)
	}
	var result searchResponse
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode Vast offer response: %w", err)
	}
	var offers []offerResponse
	if err := json.Unmarshal(result.Offers, &offers); err == nil {
		normalized := make([]Offer, len(offers))
		for index, offer := range offers {
			normalized[index] = offer.normalized()
		}
		return normalized, nil
	}
	var one offerResponse
	if err := json.Unmarshal(result.Offers, &one); err != nil {
		return nil, fmt.Errorf("decode Vast offers: %w", err)
	}
	return []Offer{one.normalized()}, nil
}
