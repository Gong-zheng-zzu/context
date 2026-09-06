package main

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/contextkeeper/service/internal/bigdata"
)

// 生成模拟的养老院生命体征数据
func main() {
	rand.Seed(time.Now().UnixNano())

	// 模拟10位老人
	residents := []string{"R001", "R002", "R003", "R004", "R005", "R006", "R007", "R008", "R009", "R010"}
	rooms := []string{"101", "102", "103", "104", "105", "201", "202", "203", "204", "205"}
	nurses := []string{"Nurse Zhang", "Nurse Li", "Nurse Wang", "Nurse Liu", "Nurse Chen"}

	// 生成过去7天的数据
	startDate := time.Now().AddDate(0, 0, -7)

	fmt.Println("=== 养老院生命体征模拟数据生成器 ===")
	fmt.Println()

	vitalSigns := make([]*bigdata.VitalSign, 0)

	for _, residentID := range residents {
		roomIdx := rand.Intn(len(rooms))

		// 每天3次测量（早中晚）
		for day := 0; day < 7; day++ {
			for timeOfDay := 0; timeOfDay < 3; timeOfDay++ {
				measureTime := startDate.AddDate(0, 0, day).Add(time.Duration(8+timeOfDay*6) * time.Hour)
				nurse := nurses[rand.Intn(len(nurses))]

				// 血压 (收缩压/舒张压)
				systolic := 110.0 + rand.Float64()*30 // 110-140
				diastolic := 70.0 + rand.Float64()*20  // 70-90
				// 10%概率异常
				if rand.Float64() < 0.1 {
					systolic += 20 // 异常高
				}

				vitalSigns = append(vitalSigns, &bigdata.VitalSign{
					ResidentID: residentID,
					Type:       bigdata.BloodPressure,
					Value:      systolic,
					Value2:     &diastolic,
					Unit:       "mmHg",
					RoomNumber: rooms[roomIdx],
					DeviceID:   "BP001",
					MeasuredAt: measureTime,
					RecordedBy: nurse,
				})

				// 体温
				temperature := 36.0 + rand.Float64()*1.5 // 36.0-37.5
				if rand.Float64() < 0.05 {
					temperature += 1.0 // 5%概率发烧
				}

				vitalSigns = append(vitalSigns, &bigdata.VitalSign{
					ResidentID: residentID,
					Type:       bigdata.Temperature,
					Value:      temperature,
					Unit:       "°C",
					RoomNumber: rooms[roomIdx],
					DeviceID:   "TEMP001",
					MeasuredAt: measureTime,
					RecordedBy: nurse,
				})

				// 心率
				heartRate := 65.0 + rand.Float64()*30 // 65-95
				if rand.Float64() < 0.08 {
					heartRate += 15 // 8%概率心率过快
				}

				vitalSigns = append(vitalSigns, &bigdata.VitalSign{
					ResidentID: residentID,
					Type:       bigdata.HeartRate,
					Value:      heartRate,
					Unit:       "bpm",
					RoomNumber: rooms[roomIdx],
					DeviceID:   "HR001",
					MeasuredAt: measureTime,
					RecordedBy: nurse,
				})

				// 血氧
				bloodOxygen := 95.0 + rand.Float64()*5 // 95-100
				if rand.Float64() < 0.05 {
					bloodOxygen -= 5 // 5%概率血氧偏低
				}

				vitalSigns = append(vitalSigns, &bigdata.VitalSign{
					ResidentID: residentID,
					Type:       bigdata.BloodOxygen,
					Value:      bloodOxygen,
					Unit:       "%",
					RoomNumber: rooms[roomIdx],
					DeviceID:   "SPO2001",
					MeasuredAt: measureTime,
					RecordedBy: nurse,
				})

				// 血糖（仅早餐前测量）
				if timeOfDay == 0 {
					bloodSugar := 4.0 + rand.Float64()*2.0 // 4.0-6.0
					if rand.Float64() < 0.1 {
						bloodSugar += 2.0 // 10%概率血糖偏高
					}

					vitalSigns = append(vitalSigns, &bigdata.VitalSign{
						ResidentID: residentID,
						Type:       bigdata.BloodSugar,
						Value:      bloodSugar,
						Unit:       "mmol/L",
						RoomNumber: rooms[roomIdx],
						DeviceID:   "GLU001",
						MeasuredAt: measureTime,
						RecordedBy: nurse,
					})
				}
			}
		}
	}

	// 输出统计信息
	fmt.Printf("生成数据统计:\n")
	fmt.Printf("- 老人数量: %d\n", len(residents))
	fmt.Printf("- 时间范围: %s 至 %s\n", startDate.Format("2006-01-02"), time.Now().Format("2006-01-02"))
	fmt.Printf("- 总记录数: %d\n", len(vitalSigns))
	fmt.Println()

	// 统计各类型数量
	typeCount := make(map[bigdata.VitalSignType]int)
	abnormalCount := 0

	for _, vs := range vitalSigns {
		typeCount[vs.Type]++
		if vs.IsAbnormal() {
			abnormalCount++
		}
	}

	fmt.Println("各类型记录数:")
	for vType, count := range typeCount {
		fmt.Printf("- %s: %d\n", vType, count)
	}
	fmt.Printf("\n异常记录数: %d (%.1f%%)\n", abnormalCount, float64(abnormalCount)/float64(len(vitalSigns))*100)
	fmt.Println()

	// 输出示例数据（前10条）
	fmt.Println("=== 示例数据（前10条）===")
	for i := 0; i < 10 && i < len(vitalSigns); i++ {
		vs := vitalSigns[i]
		abnormalMark := ""
		if vs.IsAbnormal() {
			abnormalMark = " [异常]"
		}

		if vs.Value2 != nil {
			fmt.Printf("%d. %s | %s | %.1f/%.1f %s | %s | %s%s\n",
				i+1, vs.MeasuredAt.Format("2006-01-02 15:04"), vs.Type, vs.Value, *vs.Value2, vs.Unit, vs.ResidentID, vs.RoomNumber, abnormalMark)
		} else {
			fmt.Printf("%d. %s | %s | %.1f %s | %s | %s%s\n",
				i+1, vs.MeasuredAt.Format("2006-01-02 15:04"), vs.Type, vs.Value, vs.Unit, vs.ResidentID, vs.RoomNumber, abnormalMark)
		}
	}
	fmt.Println()

	fmt.Println("=== 数据生成完成 ===")
	fmt.Println("提示: 这些数据可以通过 API 批量导入到 InfluxDB")
	fmt.Println("使用方法: 修改代码将数据通过 vitalSignService.WriteBatchVitalSigns() 写入")
}
