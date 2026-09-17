package causal_reasoning

import (
	"context"
	"testing"
)

// TestRecordDocumentSeedsPMIConfidence 验证：记录文档后语料统计非空，
// 对应实体对的 PMI 置信度非 0（此前 RecordDocument 无生产调用，语料恒空）。
func TestRecordDocumentSeedsPMIConfidence(t *testing.T) {
	extractor := NewEntityExtractor(nil)
	calc := extractor.GetPMICalculator()

	if got := calc.GetTotalDocuments(); got != 0 {
		t.Fatalf("initial total docs = %d, want 0", got)
	}

	extractor.RecordDocument([]string{"高血压", "跌倒"})

	if got := calc.GetTotalDocuments(); got != 1 {
		t.Fatalf("total docs after RecordDocument = %d, want 1", got)
	}
	score := calc.CalculatePMI("高血压", "跌倒")
	if score.Confidence <= 0 {
		t.Fatalf("PMI confidence after recording = %v, want > 0", score.Confidence)
	}
	if score.CoOccur != 1 {
		t.Fatalf("co-occurrence count = %d, want 1", score.CoOccur)
	}
}

// TestExtractWithExecutionSeedsPMICorpus 验证抽取路径自动回灌语料：
// ExtractWithExecution 成功抽取后，被抽取的 O-M-P-R 实体进入 PMI 统计，
// 后续计算 PMI 置信度时能取到非 0 分数。
func TestExtractWithExecutionSeedsPMICorpus(t *testing.T) {
	extractor := NewEntityExtractor(nil)
	text := "王爷爷患有高血压，今晨服用降压药后出现体位性低血压，随后发生跌倒。"

	relations, _, err := extractor.ExtractWithExecution(context.Background(), text, true, true, false)
	if err != nil {
		t.Fatalf("ExtractWithExecution() error = %v", err)
	}
	if len(relations) == 0 {
		t.Fatal("ExtractWithExecution() produced no relations, cannot verify corpus seeding")
	}

	// 每个被抽取的关系按一次文档观测记录。
	if got := extractor.GetPMICalculator().GetTotalDocuments(); got != len(relations) {
		t.Fatalf("total docs after extraction = %d, want %d (one per extracted relation)", got, len(relations))
	}

	// 语料就绪后，同样的实体元组应得到非 0 的 PMI 置信度。
	relation := relations[0]
	if confidence := extractor.calculatePMI(&relation); confidence <= 0 {
		t.Fatalf("PMI confidence for extracted relation = %v, want > 0 after corpus seeded", confidence)
	}
}
