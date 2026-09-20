package engines

import "testing"

// TestExtractCJKRetrievalTermsKeepsEntityBearingGrams 覆盖核心用途：中文疑问句里
// 承载实体信息的片段必须被保留，而纯虚词/疑问词片段必须被丢弃。
func TestExtractCJKRetrievalTermsKeepsEntityBearingGrams(t *testing.T) {
	terms := ExtractCJKRetrievalTerms("为什么杨桂英会出现情绪激动？", maxRetrievalTerms)
	if len(terms) == 0 {
		t.Fatal("expected CJK terms for a Chinese question, got none")
	}

	present := make(map[string]bool, len(terms))
	for _, term := range terms {
		present[term] = true
	}

	for _, wanted := range []string{"杨桂", "情绪", "激动"} {
		if !present[wanted] {
			t.Errorf("term %q should be retained, got %v", wanted, terms)
		}
	}
	for _, unwanted := range []string{"什么", "出会", "会出", "么杨"} {
		if present[unwanted] {
			t.Errorf("stop-only gram %q should be dropped, got %v", unwanted, terms)
		}
	}
}

// TestExtractCJKRetrievalTermsIgnoresNonCJK 确认纯 ASCII/标点输入不产生词元，
// 避免给英文查询注入无意义的关键词。
func TestExtractCJKRetrievalTermsIgnoresNonCJK(t *testing.T) {
	for _, input := range []string{"", "   ", "blood pressure trend", "?!,.;", "elder_025"} {
		if terms := ExtractCJKRetrievalTerms(input, maxRetrievalTerms); len(terms) != 0 {
			t.Errorf("input %q produced terms %v, want none", input, terms)
		}
	}
}

// TestExtractCJKRetrievalTermsRespectsCap 确认词元数量受上限约束，
// 避免长查询把 SQL/Cypher 参数膨胀。
func TestExtractCJKRetrievalTermsRespectsCap(t *testing.T) {
	long := "杨桂英最近七天的血压血糖心率体温体重变化趋势与用药调整记录以及情绪波动情况"
	terms := ExtractCJKRetrievalTerms(long, 12)
	if len(terms) > 12 {
		t.Fatalf("terms = %d, want <= 12", len(terms))
	}
	if len(terms) == 0 {
		t.Fatal("expected terms for a long Chinese query")
	}
}

// TestExtractCJKRetrievalTermsZeroCap 上限非法时返回空，避免调用方传 0 时行为不明确。
func TestExtractCJKRetrievalTermsZeroCap(t *testing.T) {
	if terms := ExtractCJKRetrievalTerms("杨桂英情绪激动", 0); terms != nil {
		t.Fatalf("cap=0 terms = %v, want nil", terms)
	}
}

// TestComposeCJKRetrievalTermsSkipsEmptyQueries 复现评测路径的调用形状：
// 多个查询槽位里只有部分有内容，应取第一个能抽出词元的。
func TestComposeCJKRetrievalTermsSkipsEmptyQueries(t *testing.T) {
	terms := ComposeCJKRetrievalTerms("", "   ", "为什么杨桂英会出现情绪激动？")
	if len(terms) == 0 {
		t.Fatal("expected terms from the only non-empty query")
	}

	if terms := ComposeCJKRetrievalTerms("", "   "); terms != nil {
		t.Fatalf("all-empty input terms = %v, want nil", terms)
	}
}
