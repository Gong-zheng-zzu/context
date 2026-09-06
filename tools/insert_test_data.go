package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type VitalSignRequest struct {
	ResidentID string   `json:"resident_id"`
	Type       string   `json:"type"`
	Value      float64  `json:"value"`
	Value2     *float64 `json:"value2"`
	Unit       string   `json:"unit"`
	RoomNumber string   `json:"room_number"`
	RecordedBy string   `json:"recorded_by"`
	Notes      string   `json:"notes"`
}

func float64Ptr(v float64) *float64 {
	return &v
}

func insertVitalSign(apiURL string, token string, data VitalSignRequest) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("JSON序列化失败: %v", err)
	}

	req, err := http.NewRequest("POST", apiURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("创建请求失败: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("请求失败: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("插入失败 [%d]: %s", resp.StatusCode, string(body))
	}

	fmt.Printf("✅ 插入成功: %s\n", string(body))
	return nil
}

func main() {
	apiURL := "http://localhost:8088/api/vital-signs"
	token := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJ1c2VyX2lkIjoiZG9jdG9yX3dhbmciLCJyb2xlIjoiZG9jdG9yIiwiaXNzIjoiY29udGV4dC1rZWVwZXIiLCJleHAiOjE3ODM1ODgzOTMsIm5iZiI6MTc4MzUwMTk5MywiaWF0IjoxNzgzNTAxOTkzfQ.Jtb8UBJ8ha7prVk0H99cvD_Bxl3RAiLH_NM9w10iNKk"

	fmt.Println("开始插入张奶奶的血压测试数据...")

	// 准备7天的血压数据
	bloodPressureData := []struct {
		date      string
		systolic  float64
		diastolic float64
		notes     string
	}{
		{"2026-07-02", 136, 85, "晨间测量"},
		{"2026-07-03", 135, 94, "晨间测量"},
		{"2026-07-04", 149, 91, "晨间测量，血压偏高"},
		{"2026-07-05", 131, 86, "晨间测量"},
		{"2026-07-06", 143, 85, "晨间测量"},
		{"2026-07-07", 134, 80, "晨间测量"},
		{"2026-07-08", 134, 94, "晨间测量"},
	}

	for _, bp := range bloodPressureData {
		fmt.Printf("\n插入 %s 的数据: %0.f/%0.f mmHg\n", bp.date, bp.systolic, bp.diastolic)

		req := VitalSignRequest{
			ResidentID: "张奶奶",
			Type:       "blood_pressure",
			Value:      bp.systolic,
			Value2:     float64Ptr(bp.diastolic),
			Unit:       "mmHg",
			RoomNumber: "201",
			RecordedBy: "护士小王",
			Notes:      bp.notes,
		}

		if err := insertVitalSign(apiURL, token, req); err != nil {
			fmt.Printf("❌ 错误: %v\n", err)
		}

		time.Sleep(500 * time.Millisecond) // 避免请求过快
	}

	fmt.Println("\n==================================================")
	fmt.Println("✅ 所有数据插入完成！")
	fmt.Println("现在可以访问：")
	fmt.Println("  GET http://localhost:8088/api/vital-signs/history?resident_id=张奶奶&type=blood_pressure&days=7")
	fmt.Println("来查询数据")
}
