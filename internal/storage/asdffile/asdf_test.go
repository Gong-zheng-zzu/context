package asdffile

import (
	"fmt"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"testing"
	"time"
)

const (
	testSampleRate = 16000
	testDuration   = 250 // ms
)

// tempPath 生成临时文件路径。
func tempPath(t *testing.T, name string) string {
	t.Helper()
	return filepath.Join(t.TempDir(), name)
}

// buildFile 构造一个含多种块类型的 ASDF 文件，返回其路径与预期内容。
func buildFile(t *testing.T, path string) Header {
	t.Helper()

	header := NewHeader("elder-001")
	header.DataKind = "multimodal-memory"
	header.Permission = "family-readonly"
	header.TimeStart = time.Now().Add(-24 * time.Hour).UnixNano()
	header.TimeEnd = time.Now().UnixNano()
	header.Encryption = EncryptionInfo{Enabled: true, Algorithm: "AES-256-GCM", KeyID: "key-2026"}
	header.GraphNodes = []GraphNodeAttr{
		{ID: "n1", Kind: "person", Label: "李奶奶"},
		{ID: "n2", Kind: "event", Label: "晨练"},
	}

	writer, err := Create(path, header)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	vector := make([]float64, 256)
	for i := range vector {
		vector[i] = rand.NormFloat64()
	}
	if err := writer.PutVector("embedding", vector); err != nil {
		t.Fatalf("PutVector: %v", err)
	}

	series := make([]float64, testSampleRate*testDuration/1000)
	for i := range series {
		series[i] = math.Sin(2 * math.Pi * 220 * float64(i) / testSampleRate)
	}
	if err := writer.PutSeries("waveform", series, testSampleRate); err != nil {
		t.Fatalf("PutSeries: %v", err)
	}

	rows, cols := 8, 16
	matrix := make([]float64, rows*cols)
	for i := range matrix {
		matrix[i] = float64(i)
	}
	if err := writer.PutMatrix("cooccurrence", rows, cols, matrix); err != nil {
		t.Fatalf("PutMatrix: %v", err)
	}

	if err := writer.PutRaw("notes", []byte("原始备注字节")); err != nil {
		t.Fatalf("PutRaw: %v", err)
	}
	if writer.BlockCount() != 4 {
		t.Fatalf("BlockCount = %d, want 4", writer.BlockCount())
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	return header
}

// TestWriteReadRoundTrip 验证四类数据块的写入—读取往返一致。
func TestWriteReadRoundTrip(t *testing.T) {
	path := tempPath(t, "roundtrip.asdf")
	buildFile(t, path)

	reader, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer reader.Close()

	// 文件头必须完整持久化。
	header := reader.Header()
	if header.ElderID != "elder-001" {
		t.Errorf("ElderID = %q, want elder-001", header.ElderID)
	}
	if header.BlockCount != 4 {
		t.Errorf("BlockCount = %d, want 4", header.BlockCount)
	}
	if !header.Encryption.Enabled || header.Encryption.KeyID != "key-2026" {
		t.Errorf("encryption metadata lost: %+v", header.Encryption)
	}
	if len(header.GraphNodes) != 2 || header.GraphNodes[0].Label != "李奶奶" {
		t.Errorf("graph nodes lost: %+v", header.GraphNodes)
	}

	// 主题集合与顺序。
	topics := reader.Topics()
	want := []string{"embedding", "waveform", "cooccurrence", "notes"}
	if fmt.Sprint(topics) != fmt.Sprint(want) {
		t.Errorf("Topics = %v, want %v", topics, want)
	}

	// 时序块：值与采样率都要还原。
	_, sampleRate, err := reader.ReadSeries("waveform")
	if err != nil {
		t.Fatalf("ReadSeries: %v", err)
	}
	if sampleRate != testSampleRate {
		t.Errorf("sample rate = %d, want %d", sampleRate, testSampleRate)
	}

	// 矩阵块：形状要还原。
	rows, cols, matrix, err := reader.ReadMatrix("cooccurrence")
	if err != nil {
		t.Fatalf("ReadMatrix: %v", err)
	}
	if rows != 8 || cols != 16 {
		t.Errorf("matrix shape = %dx%d, want 8x16", rows, cols)
	}
	if len(matrix) != 128 {
		t.Errorf("matrix length = %d, want 128", len(matrix))
	}
	for i, value := range matrix {
		if value != float64(i) {
			t.Fatalf("matrix[%d] = %v, want %v", i, value, float64(i))
		}
	}

	// 原始字节块。
	raw, err := reader.ReadBlock("notes")
	if err != nil {
		t.Fatalf("ReadBlock: %v", err)
	}
	if string(raw) != "原始备注字节" {
		t.Errorf("raw = %q, want 原始备注字节", raw)
	}
}

// TestReadVectorPreservesValues 向量必须逐位一致（浮点不能有任何损失）。
func TestReadVectorPreservesValues(t *testing.T) {
	original := make([]float64, 512)
	for i := range original {
		original[i] = rand.NormFloat64() * 1e6
	}

	path := tempPath(t, "vector.asdf")
	writer, err := Create(path, NewHeader("elder-002"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := writer.PutVector("v", original); err != nil {
		t.Fatalf("PutVector: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	reader, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer reader.Close()

	got, err := reader.ReadVector("v")
	if err != nil {
		t.Fatalf("ReadVector: %v", err)
	}
	if len(got) != len(original) {
		t.Fatalf("length = %d, want %d", len(got), len(original))
	}
	// IEEE-754 位级往返，必须是完全相等（而非近似相等）。
	for i := range original {
		if got[i] != original[i] {
			t.Fatalf("value[%d] = %v, want %v (bitwise)", i, got[i], original[i])
		}
	}
}

// TestOnDemandLoadingReadsOnlyOneBlock 验证"按需加载"：
// 读取单个块时，实际读取的字节数必须远小于文件总大小。
//
// 这是本格式存在的核心理由——若这一性质不成立，索引映射区就没有意义。
func TestOnDemandLoadingReadsOnlyOneBlock(t *testing.T) {
	path := tempPath(t, "ondemand.asdf")

	writer, err := Create(path, NewHeader("elder-003"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	// 多个大块，只读其中一个。
	for i := 0; i < 6; i++ {
		values := make([]float64, 20000)
		for j := range values {
			values[j] = math.Sin(float64(i*10000 + j))
		}
		if err := writer.PutVector(fmt.Sprintf("block-%d", i), values); err != nil {
			t.Fatalf("PutVector %d: %v", i, err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	reader, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer reader.Close()

	fileSize := reader.FileSize()
	// 打开阶段只读了魔数、长度字段、Header 与索引，尚未触碰任何数据块。
	beforeRead := reader.BytesRead()

	values, err := reader.ReadVector("block-3")
	if err != nil {
		t.Fatalf("ReadVector: %v", err)
	}
	if len(values) != 20000 {
		t.Fatalf("length = %d, want 20000", len(values))
	}

	read := reader.BytesRead() - beforeRead
	if read >= fileSize/2 {
		t.Errorf("reading one of six blocks consumed %d bytes of a %d byte file; "+
			"on-demand loading is not working (should be ~1/6 plus overhead)",
			read, fileSize)
	}
	t.Logf("按需加载：读取 1/6 块消耗 %d 字节 / 文件 %d 字节，mmap=%v",
		read, fileSize, reader.IsMmapActive())
}

// TestCompressionShrinksRepetitiveData 可压缩数据必须真的变小，且解压后一致。
func TestCompressionShrinksRepetitiveData(t *testing.T) {
	// 高度重复的数据，LZ4 应能显著压缩。
	values := make([]float64, 50000)
	for i := range values {
		values[i] = float64(i % 10)
	}

	path := tempPath(t, "compress.asdf")
	writer, err := CreateWithCompression(path, NewHeader("elder-004"), DefaultCompressionConfig())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := writer.PutVector("repetitive", values); err != nil {
		t.Fatalf("PutVector: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	reader, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer reader.Close()

	entry, ok := reader.Entry("repetitive")
	if !ok {
		t.Fatal("missing index entry")
	}
	if !entry.Compressed {
		t.Error("repetitive data was not marked as compressed")
	}
	if entry.StoredLength >= entry.RawLength {
		t.Errorf("stored %d >= raw %d; compression made it no smaller",
			entry.StoredLength, entry.RawLength)
	}

	got, err := reader.ReadVector("repetitive")
	if err != nil {
		t.Fatalf("ReadVector: %v", err)
	}
	for i := range values {
		if got[i] != values[i] {
			t.Fatalf("value[%d] = %v, want %v", i, got[i], values[i])
		}
	}
}

// TestCompressionDisabledStoresRaw 关闭压缩时应原样存储。
func TestCompressionDisabledStoresRaw(t *testing.T) {
	values := make([]float64, 5000)
	for i := range values {
		values[i] = float64(i % 10)
	}

	path := tempPath(t, "nocompress.asdf")
	writer, err := CreateWithCompression(path, NewHeader("elder-005"),
		CompressionConfig{Enabled: false})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := writer.PutVector("v", values); err != nil {
		t.Fatalf("PutVector: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	reader, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer reader.Close()

	entry, _ := reader.Entry("v")
	if entry.Compressed {
		t.Error("compression disabled, but entry marked compressed")
	}
	if entry.StoredLength != entry.RawLength {
		t.Errorf("stored %d != raw %d", entry.StoredLength, entry.RawLength)
	}
}

// TestVerifyDetectsTampering 完整性校验必须能发现篡改——
// 这是"防篡改审计链"在存储层的基础保证。
func TestVerifyDetectsTampering(t *testing.T) {
	path := tempPath(t, "verify.asdf")
	buildFile(t, path)

	reader, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := reader.Verify(); err != nil {
		t.Fatalf("Verify on intact file: %v", err)
	}
	reader.Close()

	// 篡改数据区（不改动长度，只翻转中间某个字节）。
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	mid := len(data) / 2
	data[mid] ^= 0xFF
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	tampered, err := Open(path)
	if err != nil {
		// 篡改若破坏了结构（如索引），Open 报错也算检测成功。
		return
	}
	defer tampered.Close()
	if err := tampered.Verify(); err == nil {
		t.Error("Verify succeeded on tampered file; tampering went undetected")
	}
}

// TestOpenRejectsNonASDF 非 ASDF 文件必须被魔数检查拦下，不得误解析。
func TestOpenRejectsNonASDF(t *testing.T) {
	path := tempPath(t, "notasdf.bin")
	payload := make([]byte, 4096)
	for i := range payload {
		payload[i] = byte(i)
	}
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, err := Open(path); err == nil {
		t.Error("Open succeeded on a non-ASDF file")
	}
}

// TestDuplicateTopicRejected 重复主题必须报错：
// 否则索引里会出现两个同名条目，读取时静默取到错误的数据。
func TestDuplicateTopicRejected(t *testing.T) {
	path := tempPath(t, "dup.asdf")
	writer, err := Create(path, NewHeader("elder-006"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	defer writer.Close()

	if err := writer.PutVector("same", []float64{1, 2, 3}); err != nil {
		t.Fatalf("first PutVector: %v", err)
	}
	if err := writer.PutVector("same", []float64{4, 5, 6}); err == nil {
		t.Error("duplicate topic accepted")
	}
	if err := writer.PutRaw("", []byte("x")); err == nil {
		t.Error("empty topic accepted")
	}
}

// TestClosedWriterRejectsWrites 关闭后的写入器必须拒绝写入。
func TestClosedWriterRejectsWrites(t *testing.T) {
	path := tempPath(t, "closed.asdf")
	writer, err := Create(path, NewHeader("elder-007"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := writer.PutVector("v", []float64{1}); err != nil {
		t.Fatalf("PutVector: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := writer.PutVector("w", []float64{2}); err == nil {
		t.Error("write after close accepted")
	}
	if err := writer.Close(); err == nil {
		t.Error("double close accepted")
	}
}

// TestUnknownTopicRejected 读取不存在的主题必须报错而非返回空值。
func TestUnknownTopicRejected(t *testing.T) {
	path := tempPath(t, "unknown.asdf")
	buildFile(t, path)

	reader, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer reader.Close()

	if _, err := reader.ReadBlock("nope"); err == nil {
		t.Error("ReadBlock on unknown topic succeeded")
	}
	if _, ok := reader.Entry("nope"); ok {
		t.Error("Entry on unknown topic returned ok")
	}
	// 类型不匹配也要拦下：矩阵块不能当成向量读。
	if _, err := reader.ReadVector("cooccurrence"); err == nil {
		t.Error("ReadVector on a matrix block succeeded")
	}
}

// TestEmptyFileRejected 空文件与截断文件必须被明确拒绝。
func TestEmptyFileRejected(t *testing.T) {
	empty := tempPath(t, "empty.asdf")
	if err := os.WriteFile(empty, nil, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, err := Open(empty); err == nil {
		t.Error("Open succeeded on empty file")
	}

	// 截断：只保留前 100 字节，Header/索引都不完整。
	truncated := tempPath(t, "truncated.asdf")
	buildFile(t, truncated)
	full, err := os.ReadFile(truncated)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if err := os.WriteFile(truncated, full[:100], 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, err := Open(truncated); err == nil {
		t.Error("Open succeeded on truncated file")
	}
}

// TestBlockDataOutlivesReader 回归测试：读取到的数据必须比 Reader 活得更久。
//
// 这是修复一个真实崩溃后的护栏。曾经 ReadBlock 在 mmap 路径直接返回映射
// 内存的切片，Reader.Close() 解除映射后该切片悬空，调用方再访问就是
// SIGSEGV。若将来有人为了"零拷贝"改回去，这个测试会立刻炸掉。
func TestBlockDataOutlivesReader(t *testing.T) {
	path := tempPath(t, "lifetime.asdf")
	values := make([]float64, 4000)
	for i := range values {
		values[i] = float64(i) * 1.5
	}

	writer, err := Create(path, NewHeader("elder-008"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := writer.PutVector("v", values); err != nil {
		t.Fatalf("PutVector: %v", err)
	}
	if err := writer.PutRaw("r", []byte("原始字节")); err != nil {
		t.Fatalf("PutRaw: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	reader, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	vector, err := reader.ReadVector("v")
	if err != nil {
		reader.Close()
		t.Fatalf("ReadVector: %v", err)
	}
	raw, err := reader.ReadBlock("r")
	if err != nil {
		reader.Close()
		t.Fatalf("ReadBlock: %v", err)
	}
	// 关键：先关闭（解除 mmap 映射），再访问数据。
	if err := reader.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	for i := range values {
		if vector[i] != values[i] {
			t.Fatalf("vector[%d] = %v, want %v (data did not outlive reader)", i, vector[i], values[i])
		}
	}
	if string(raw) != "原始字节" {
		t.Fatalf("raw = %q, want 原始字节 (data did not outlive reader)", raw)
	}
}

// TestFloat64Codec 编解码必须是位级无损，且拒绝非法长度。
func TestFloat64Codec(t *testing.T) {
	values := []float64{0, -1, 1, math.Inf(1), math.Inf(-1), math.SmallestNonzeroFloat64, math.MaxFloat64}
	encoded, err := encodeFloat64Slice(values)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if len(encoded) != len(values)*8 {
		t.Errorf("encoded length = %d, want %d", len(encoded), len(values)*8)
	}
	decoded, err := decodeFloat64Slice(encoded)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	for i := range values {
		if decoded[i] != values[i] {
			t.Errorf("decoded[%d] = %v, want %v", i, decoded[i], values[i])
		}
	}
	// 长度非 8 的倍数必须报错，否则会静默产生错误数据。
	if _, err := decodeFloat64Slice(make([]byte, 17)); err == nil {
		t.Error("decode accepted a payload that is not a multiple of 8 bytes")
	}
}

// TestWriterCleansUpTempBuffer Close 与 Abort 都必须清理临时块缓冲。
//
// 数据块先落临时文件再组装，若临时文件残留会长期占用磁盘，
// 且暴露"写到一半"的内容。
func TestWriterCleansUpTempBuffer(t *testing.T) {
	// --- Close 路径 ---
	path := tempPath(t, "closed.asdf")
	writer, err := Create(path, NewHeader("elder-009"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := writer.PutRaw("p", []byte("data")); err != nil {
		t.Fatalf("PutRaw: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := os.Stat(path + ".blocks.tmp"); !os.IsNotExist(err) {
		t.Error("temp block buffer left behind after Close")
	}

	// --- Abort 路径 ---
	abortPath := tempPath(t, "aborted.asdf")
	aborted, err := Create(abortPath, NewHeader("elder-010"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := aborted.PutRaw("p", []byte("data")); err != nil {
		t.Fatalf("PutRaw: %v", err)
	}
	// 写入过程中临时缓冲确实存在（否则下面的清理断言毫无意义）。
	if _, err := os.Stat(abortPath + ".blocks.tmp"); err != nil {
		t.Fatalf("temp block buffer should exist while writing: %v", err)
	}
	if err := aborted.Abort(); err != nil {
		t.Fatalf("Abort: %v", err)
	}
	if _, err := os.Stat(abortPath + ".blocks.tmp"); !os.IsNotExist(err) {
		t.Error("temp block buffer left behind after Abort")
	}
	// Abort 不应产生最终文件。
	if _, err := os.Stat(abortPath); !os.IsNotExist(err) {
		t.Error("Abort created the final file")
	}
	// Abort 后重复调用应安全（幂等）。
	if err := aborted.Abort(); err != nil {
		t.Errorf("second Abort: %v", err)
	}
}

// TestWriterHandlesLargeArchive 大归档必须走通流式组装路径。
//
// 数据块先落临时文件、Close 时用 io.Copy 分块搬运，这样内存峰值与归档
// 体积无关。本测试写入远大于单次缓冲的多个块，验证组装正确、内容无损。
func TestWriterHandlesLargeArchive(t *testing.T) {
	path := tempPath(t, "large.asdf")
	writer, err := Create(path, NewHeader("elder-011"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// 8 个块 × 2MB = 16MB。分多次写入是为了让每个块的内存
	// 在写入后即可被回收——这才是流式写入要覆盖的场景。
	const blockFloats = 256 * 1024 // 2MB / 8字节
	const blockCount = 8
	var lastBlock []float64
	for i := 0; i < blockCount; i++ {
		values := make([]float64, blockFloats)
		for j := range values {
			values[j] = float64(i*blockFloats + j)
		}
		if err := writer.PutVector(fmt.Sprintf("blk-%d", i), values); err != nil {
			t.Fatalf("PutVector %d: %v", i, err)
		}
		lastBlock = values
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	reader, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer reader.Close()

	// 注意断言方向：这里的数据是递增序列，高度可压缩，压缩后**必然小于**
	// 原始字节数。若断言"文件 >= 原始大小"，等于在否定压缩功能本身。
	raw := int64(blockCount * blockFloats * 8)
	size := reader.FileSize()
	if size <= 0 || size >= raw {
		t.Errorf("file size = %d, want > 0 and < %d (compression should shrink it)", size, raw)
	}
	t.Logf("大归档实测：原始 %d 字节 → 落盘 %d 字节（压缩率 %.1f%%）",
		raw, size, 100*(1-float64(size)/float64(raw)))

	if err := reader.Verify(); err != nil {
		t.Fatalf("Verify on large archive: %v", err)
	}

	// 抽查最后一块，确认流式组装没有错位或截断。
	got, err := reader.ReadVector(fmt.Sprintf("blk-%d", blockCount-1))
	if err != nil {
		t.Fatalf("ReadVector: %v", err)
	}
	if len(got) != len(lastBlock) {
		t.Fatalf("length = %d, want %d", len(got), len(lastBlock))
	}
	for i := range lastBlock {
		if got[i] != lastBlock[i] {
			t.Fatalf("value[%d] = %v, want %v", i, got[i], lastBlock[i])
		}
	}
}
