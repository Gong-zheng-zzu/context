#!/bin/bash
# 插入测试生命体征数据（血压）

API_URL="http://localhost:8088/api/vital-signs"
TOKEN="your-jwt-token"  # 需要替换为实际的JWT token

# 张奶奶的血压数据（最近7天）
echo "正在插入张奶奶的血压数据..."

# 07-02: 136/85
curl -X POST "$API_URL" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{
    "resident_id": "张奶奶",
    "type": "blood_pressure",
    "value": 136,
    "value2": 85,
    "unit": "mmHg",
    "room_number": "201",
    "recorded_by": "护士小王",
    "notes": "晨间测量"
  }'

sleep 1

# 07-03: 135/94
curl -X POST "$API_URL" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{
    "resident_id": "张奶奶",
    "type": "blood_pressure",
    "value": 135,
    "value2": 94,
    "unit": "mmHg",
    "room_number": "201",
    "recorded_by": "护士小王",
    "notes": "晨间测量"
  }'

sleep 1

# 07-04: 149/91
curl -X POST "$API_URL" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{
    "resident_id": "张奶奶",
    "type": "blood_pressure",
    "value": 149,
    "value2": 91,
    "unit": "mmHg",
    "room_number": "201",
    "recorded_by": "护士小王",
    "notes": "晨间测量，血压偏高"
  }'

sleep 1

# 07-05: 131/86
curl -X POST "$API_URL" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{
    "resident_id": "张奶奶",
    "type": "blood_pressure",
    "value": 131,
    "value2": 86,
    "unit": "mmHg",
    "room_number": "201",
    "recorded_by": "护士小王",
    "notes": "晨间测量"
  }'

sleep 1

# 07-06: 143/85
curl -X POST "$API_URL" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{
    "resident_id": "张奶奶",
    "type": "blood_pressure",
    "value": 143,
    "value2": 85,
    "unit": "mmHg",
    "room_number": "201",
    "recorded_by": "护士小王",
    "notes": "晨间测量"
  }'

sleep 1

# 07-07: 134/80
curl -X POST "$API_URL" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{
    "resident_id": "张奶奶",
    "type": "blood_pressure",
    "value": 134,
    "value2": 80,
    "unit": "mmHg",
    "room_number": "201",
    "recorded_by": "护士小王",
    "notes": "晨间测量"
  }'

sleep 1

# 07-08: 134/94
curl -X POST "$API_URL" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{
    "resident_id": "张奶奶",
    "type": "blood_pressure",
    "value": 134,
    "value2": 94,
    "unit": "mmHg",
    "room_number": "201",
    "recorded_by": "护士小王",
    "notes": "晨间测量"
  }'

echo ""
echo "✅ 数据插入完成！"
echo "现在可以调用 GET /api/vital-signs/history?resident_id=张奶奶&type=blood_pressure&days=7 查询数据"
