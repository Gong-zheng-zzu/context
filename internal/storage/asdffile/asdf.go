// Package asdffile 实现 ASDF（Advanced Scientific Data Format）一体化文件序列化。
//
// 策划书 2.5.2 节描述的四段式结构：
//
//	YAML Header   —— Schema 版本、老人 ID、时间范围、数据类型、索引 Offset、
//	                 图谱节点属性、加密标识
//	BinaryBlocks  —— 高维向量数组、时序波形片段、批量数值矩阵，
//	                 贴近原生数组布局，规避 JSON/XML 的字符串化开销
//	索引映射区     —— 逻辑主题 → 物理 Offset，使局部读取无需加载整个文件
//	校验与加密区   —— SHA-256 摘要、加密标识、权限标签
//
// 命名澄清
// --------
// 项目中已存在 `internal/security/asdf_framework.go`，那是**对抗样本防御
// 框架**（Adversarial Sample Defense Framework）。本包的 ASDF 是
// **Advanced Scientific Data Format**，两者同名但完全无关。
//
// 文件布局
// --------
//
//	┌────────────────────────────────────┐
//	│ magic        8 B   "ASDFv1\x00\x00" │
//	│ headerLen    8 B   uint64 LE        │
//	│ header       …     YAML             │
//	│ indexLen     8 B   uint64 LE        │
//	│ index        …     二进制索引条目    │
//	│ blocks       …     数据块（可压缩）  │
//	│ digest      32 B   SHA-256          │
//	└────────────────────────────────────┘
//
// 把 header 与 index 前置、blocks 后置，是为了让"打开 → 定位 → 读取单个块"
// 全程只需 O(1) 次小读取，这正是按需加载的前提。
//
// 边界声明
// --------
//   - Writer 在**内存中**累积数据块，Close 时一次性落盘。适用于单个归档
//     文件规模可控的场景；若要写入超过可用内存的超大归档，需要改为
//     临时文件缓冲，本版本未实现。
//   - Header 的 Encryption 字段只记录**加密元数据**（算法、密钥 ID），
//     本包不做实际加解密——加解密由 `internal/security` 负责，
//     本包不重复实现密码学，避免"两套加密"带来的密钥管理风险。
package asdffile

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"os"
	"time"

	"github.com/pierrec/lz4/v4"
	"gopkg.in/yaml.v3"
)

// Magic 文件魔数，含版本。
var Magic = []byte{'A', 'S', 'D', 'F', 'v', '1', 0, 0}

const (
	// magicSize 魔数字节数。
	magicSize = 8
	// lengthFieldSize uint64 长度字段的字节数。
	lengthFieldSize = 8
	// digestSize SHA-256 摘要长度。
	digestSize = 32
)

// BlockKind 数据块类型。
type BlockKind uint32

const (
	// KindRaw 原始字节。
	KindRaw BlockKind = 1
	// KindVector 一维 float64 向量（高维嵌入等）。
	KindVector BlockKind = 2
	// KindSeries 时序波形片段。
	KindSeries BlockKind = 3
	// KindMatrix 二维数值矩阵（行优先）。
	KindMatrix BlockKind = 4
)

// String 返回块类型的可读名称。
func (k BlockKind) String() string {
	switch k {
	case KindRaw:
		return "raw"
	case KindVector:
		return "vector"
	case KindSeries:
		return "series"
	case KindMatrix:
		return "matrix"
	default:
		return fmt.Sprintf("unknown(%d)", uint32(k))
	}
}

// EncryptionInfo 加密元数据（不含密钥本身）。
type EncryptionInfo struct {
	Enabled   bool   `yaml:"enabled"`
	Algorithm string `yaml:"algorithm,omitempty"`
	KeyID     string `yaml:"key_id,omitempty"`
}

// GraphNodeAttr 图谱节点属性，写入 Header 以便归档可自描述。
type GraphNodeAttr struct {
	ID    string `yaml:"id"`
	Kind  string `yaml:"kind"`
	Label string `yaml:"label,omitempty"`
}

// Header ASDF 文件头（YAML 序列化）。
type Header struct {
	Schema     string          `yaml:"schema"`
	Version    int             `yaml:"version"`
	ElderID    string          `yaml:"elder_id"`
	TimeStart  int64           `yaml:"time_start,omitempty"`
	TimeEnd    int64           `yaml:"time_end,omitempty"`
	DataKind   string          `yaml:"data_kind,omitempty"`
	BlockCount int             `yaml:"block_count"`
	Permission string          `yaml:"permission,omitempty"`
	CreatedAt  int64           `yaml:"created_at"`
	Encryption EncryptionInfo  `yaml:"encryption"`
	GraphNodes []GraphNodeAttr `yaml:"graph_nodes,omitempty"`
}

// NewHeader 构造带默认值的文件头。
func NewHeader(elderID string) Header {
	return Header{
		Schema:    "asdf",
		Version:   1,
		ElderID:   elderID,
		CreatedAt: time.Now().UnixNano(),
	}
}

// IndexEntry 索引映射区的一条记录：逻辑主题 → 物理 Offset。
type IndexEntry struct {
	Topic        string
	Kind         BlockKind
	Offset       uint64 // 相对 blocks 区起点
	RawLength    uint64 // 解压后字节数
	StoredLength uint64 // 实际落盘字节数
	Compressed   bool
	ItemCount    uint64 // 元素个数（向量长度 / 采样点数 / 矩阵总元素数）
	Dimension    uint32 // 单元素维度（向量维度 / 矩阵列数）
}

// ============================================================================
// 索引编解码
// ============================================================================

// encodeIndex 把索引条目序列化为二进制。
func encodeIndex(entries []IndexEntry) ([]byte, error) {
	var buffer bytes.Buffer
	for _, entry := range entries {
		if err := writeString(&buffer, entry.Topic); err != nil {
			return nil, err
		}
		fields := []interface{}{
			uint32(entry.Kind),
			entry.Offset,
			entry.RawLength,
			entry.StoredLength,
			entry.ItemCount,
			entry.Dimension,
		}
		for _, field := range fields {
			if err := binary.Write(&buffer, binary.LittleEndian, field); err != nil {
				return nil, err
			}
		}
		if entry.Compressed {
			if err := buffer.WriteByte(1); err != nil {
				return nil, err
			}
		} else {
			if err := buffer.WriteByte(0); err != nil {
				return nil, err
			}
		}
	}
	return buffer.Bytes(), nil
}

// decodeIndex 解析二进制索引。
func decodeIndex(data []byte) ([]IndexEntry, error) {
	reader := bytes.NewReader(data)
	var entries []IndexEntry
	for reader.Len() > 0 {
		topic, err := readString(reader)
		if err != nil {
			return nil, fmt.Errorf("asdffile: read topic: %w", err)
		}
		var kind uint32
		var offset, rawLength, storedLength, itemCount uint64
		var dimension uint32
		targets := []interface{}{&kind, &offset, &rawLength, &storedLength, &itemCount, &dimension}
		for _, target := range targets {
			if err := binary.Read(reader, binary.LittleEndian, target); err != nil {
				return nil, fmt.Errorf("asdffile: read index field: %w", err)
			}
		}
		flag, err := reader.ReadByte()
		if err != nil {
			return nil, fmt.Errorf("asdffile: read compression flag: %w", err)
		}
		entries = append(entries, IndexEntry{
			Topic:        topic,
			Kind:         BlockKind(kind),
			Offset:       offset,
			RawLength:    rawLength,
			StoredLength: storedLength,
			ItemCount:    itemCount,
			Dimension:    dimension,
			Compressed:   flag == 1,
		})
	}
	return entries, nil
}

// writeString 写入长度前缀字符串（uint32 长度 + 字节）。
func writeString(buffer *bytes.Buffer, value string) error {
	if len(value) > math.MaxUint32 {
		return fmt.Errorf("asdffile: string too long: %d", len(value))
	}
	if err := binary.Write(buffer, binary.LittleEndian, uint32(len(value))); err != nil {
		return err
	}
	_, err := buffer.WriteString(value)
	return err
}

// readString 读取长度前缀字符串。
func readString(reader io.Reader) (string, error) {
	var length uint32
	if err := binary.Read(reader, binary.LittleEndian, &length); err != nil {
		return "", err
	}
	if length == 0 {
		return "", nil
	}
	data := make([]byte, length)
	if _, err := io.ReadFull(reader, data); err != nil {
		return "", err
	}
	return string(data), nil
}

// ============================================================================
// 压缩（LZ4，块级）
// ============================================================================

// CompressionConfig 压缩配置。
type CompressionConfig struct {
	// Enabled 是否启用压缩。
	Enabled bool
	// MinSize 触发压缩的最小原始字节数。
	// 过小的数据块压缩收益为负（甚至变大），不如直接存。
	MinSize int
}

// DefaultCompressionConfig 默认压缩配置：启用，且仅对 ≥1KB 的块压缩。
func DefaultCompressionConfig() CompressionConfig {
	return CompressionConfig{Enabled: true, MinSize: 1024}
}

// compressBlock 用 LZ4 压缩一个数据块。
func compressBlock(data []byte) ([]byte, error) {
	var buffer bytes.Buffer
	writer := lz4.NewWriter(&buffer)
	if _, err := writer.Write(data); err != nil {
		return nil, err
	}
	// 必须 Close：LZ4 的帧尾在此写出，缺失会导致解压端报错。
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

// decompressBlock 解压一个 LZ4 数据块。
func decompressBlock(data []byte) ([]byte, error) {
	reader := lz4.NewReader(bytes.NewReader(data))
	return io.ReadAll(reader)
}

// maybeCompress 按配置决定是否压缩，返回待落盘数据与是否压缩。
//
// 压缩后反而更大时放弃压缩：LZ4 对已有的高熵数据（如随机向量）常常
// 无收益，强行压缩只会白白增加解压开销。
func maybeCompress(data []byte, config CompressionConfig) ([]byte, bool, error) {
	if !config.Enabled || len(data) < config.MinSize {
		return data, false, nil
	}
	compressed, err := compressBlock(data)
	if err != nil {
		return nil, false, fmt.Errorf("asdffile: compress: %w", err)
	}
	if len(compressed) >= len(data) {
		return data, false, nil
	}
	return compressed, true, nil
}

// ============================================================================
// Writer
// ============================================================================

// Writer 构建 ASDF 文件。
//
// 非并发安全。使用流程：Create → Put* → Close。
// Close 之前文件尚未完整，不得被 Reader 打开。
type Writer struct {
	path     string
	header   Header
	entries  []IndexEntry
	temp     *os.File
	tempPath string
	// blocksSize 已缓冲的数据块总字节数，即下一个块的 Offset。
	blocksSize  int64
	compression CompressionConfig
	closed      bool
}

// Create 创建一个 ASDF 文件写入器。
func Create(path string, header Header) (*Writer, error) {
	return CreateWithCompression(path, header, DefaultCompressionConfig())
}

// CreateWithCompression 以指定压缩配置创建写入器。
func CreateWithCompression(path string, header Header, compression CompressionConfig) (*Writer, error) {
	if path == "" {
		return nil, fmt.Errorf("asdffile: empty path")
	}
	if header.Schema == "" {
		header.Schema = "asdf"
	}
	if header.Version == 0 {
		header.Version = 1
	}

	// 数据块先写入临时文件，而不是累积在内存里。
	//
	// 原因：归档条目可能很大（长音频、高维向量集合）。若把所有数据块
	// 缓存在 bytes.Buffer 中，内存占用会随归档体积线性增长，极端情况下
	// 直接 OOM——而"归档"这个动作本身往往正是内存吃紧时才会做的。
	// 写入临时文件后，Close 时只需顺序复制，峰值内存降为常数级。
	tempPath := path + ".blocks.tmp"
	temp, err := os.Create(tempPath)
	if err != nil {
		return nil, fmt.Errorf("asdffile: create block buffer %q: %w", tempPath, err)
	}

	return &Writer{
		path:        path,
		header:      header,
		compression: compression,
		temp:        temp,
		tempPath:    tempPath,
	}, nil
}

// Abort 放弃本次写入并清理临时文件。
//
// 写入器创建后若既未 Close 也未 Abort，临时文件会残留在磁盘上。
// 提供该方法让"中途放弃"成为显式且干净的操作。
func (w *Writer) Abort() error {
	if w.closed {
		return nil
	}
	w.closed = true
	if w.temp != nil {
		w.temp.Close()
	}
	if w.tempPath != "" {
		return os.Remove(w.tempPath)
	}
	return nil
}

// PutRaw 写入一个原始字节块。
func (w *Writer) PutRaw(topic string, payload []byte) error {
	return w.put(topic, KindRaw, payload, uint64(len(payload)), 0)
}

// PutVector 写入一个 float64 向量。
func (w *Writer) PutVector(topic string, values []float64) error {
	payload, err := encodeFloat64Slice(values)
	if err != nil {
		return err
	}
	return w.put(topic, KindVector, payload, uint64(len(values)), 0)
}

// PutSeries 写入一段时序波形。
//
// 采样率随块一并存储（作为 Dimension 字段复用），避免读取端需要外部
// 元数据才能解释波形。
func (w *Writer) PutSeries(topic string, values []float64, sampleRate int) error {
	payload, err := encodeFloat64Slice(values)
	if err != nil {
		return err
	}
	return w.put(topic, KindSeries, payload, uint64(len(values)), uint32(sampleRate))
}

// PutMatrix 写入一个行优先矩阵。
func (w *Writer) PutMatrix(topic string, rows, cols int, values []float64) error {
	if rows <= 0 || cols <= 0 {
		return fmt.Errorf("asdffile: invalid matrix shape %dx%d", rows, cols)
	}
	if len(values) != rows*cols {
		return fmt.Errorf("asdffile: matrix size mismatch: %d values for %dx%d", len(values), rows, cols)
	}
	payload, err := encodeFloat64Slice(values)
	if err != nil {
		return err
	}
	return w.put(topic, KindMatrix, payload, uint64(len(values)), uint32(cols))
}

// put 追加一个数据块（可能压缩）并记录索引条目。
func (w *Writer) put(topic string, kind BlockKind, payload []byte, itemCount uint64, dimension uint32) error {
	if w.closed {
		return fmt.Errorf("asdffile: writer already closed")
	}
	if topic == "" {
		return fmt.Errorf("asdffile: empty topic")
	}
	for _, existing := range w.entries {
		if existing.Topic == topic {
			return fmt.Errorf("asdffile: duplicate topic %q", topic)
		}
	}

	stored, compressed, err := maybeCompress(payload, w.compression)
	if err != nil {
		return err
	}

	offset := uint64(w.blocksSize)
	if _, err := w.temp.Write(stored); err != nil {
		return fmt.Errorf("asdffile: write block buffer: %w", err)
	}
	w.blocksSize += int64(len(stored))
	w.entries = append(w.entries, IndexEntry{
		Topic:        topic,
		Kind:         kind,
		Offset:       offset,
		RawLength:    uint64(len(payload)),
		StoredLength: uint64(len(stored)),
		ItemCount:    itemCount,
		Dimension:    dimension,
		Compressed:   compressed,
	})
	return nil
}

// BlockCount 返回已写入的块数。
func (w *Writer) BlockCount() int { return len(w.entries) }

// Close 组装并写入文件，计算并写入 SHA-256 摘要。
func (w *Writer) Close() error {
	if w.closed {
		return fmt.Errorf("asdffile: writer already closed")
	}
	w.closed = true

	// 先收尾临时块缓冲：后续只需顺序读取它来组装最终文件。
	if w.temp != nil {
		if err := w.temp.Close(); err != nil {
			return fmt.Errorf("asdffile: close block buffer: %w", err)
		}
	}
	// 无论组装成功与否，临时缓冲都不再需要。
	defer os.Remove(w.tempPath)

	w.header.BlockCount = len(w.entries)

	headerBytes, err := yaml.Marshal(&w.header)
	if err != nil {
		return fmt.Errorf("asdffile: marshal header: %w", err)
	}
	indexBytes, err := encodeIndex(w.entries)
	if err != nil {
		return fmt.Errorf("asdffile: encode index: %w", err)
	}

	file, err := os.Create(w.path)
	if err != nil {
		return fmt.Errorf("asdffile: create %q: %w", w.path, err)
	}
	defer file.Close()

	digest := sha256.New()
	multi := io.MultiWriter(file, digest)

	writeAll := func(data []byte) error {
		if _, err := multi.Write(data); err != nil {
			return fmt.Errorf("asdffile: write: %w", err)
		}
		return nil
	}
	writeUint64 := func(value uint64) error {
		var buffer [lengthFieldSize]byte
		binary.LittleEndian.PutUint64(buffer[:], value)
		return writeAll(buffer[:])
	}

	if err := writeAll(Magic); err != nil {
		return err
	}
	if err := writeUint64(uint64(len(headerBytes))); err != nil {
		return err
	}
	if err := writeAll(headerBytes); err != nil {
		return err
	}
	if err := writeUint64(uint64(len(indexBytes))); err != nil {
		return err
	}
	if err := writeAll(indexBytes); err != nil {
		return err
	}
	// 数据块区：从临时文件流式复制。
	//
	// 必须用 io.Copy 分块搬运，不能把整个数据区读进内存再写——
	// 那样临时文件缓冲就白做了，内存峰值仍会随归档体积线性增长。
	blocks, err := os.Open(w.tempPath)
	if err != nil {
		return fmt.Errorf("asdffile: reopen block buffer: %w", err)
	}
	defer blocks.Close()
	if _, err := io.Copy(multi, blocks); err != nil {
		return fmt.Errorf("asdffile: write blocks: %w", err)
	}
	// 摘要覆盖前面所有内容（不含摘要自身）。
	if err := writeAll(digest.Sum(nil)); err != nil {
		return err
	}
	return file.Sync()
}

// ============================================================================
// Reader
// ============================================================================

// Reader 读取 ASDF 文件。
//
// 支持两种访问方式：
//   - mmap（默认尝试）：整个文件映射到地址空间，读块时零拷贝切片；
//   - ReadAt 回退：mmap 不可用时按偏移读取，功能完全一致。
//
// 通过 BytesRead 可以验证"读单个块时并未加载整个文件"。
type Reader struct {
	path      string
	file      *os.File
	mapping   *mappedFile
	header    Header
	entries   []IndexEntry
	byTopic   map[string]*IndexEntry
	dataStart int64
	fileSize  int64
	bytesRead int64
	closed    bool
}

// Open 打开 ASDF 文件。
//
// 会解析魔数、Header 与索引，但不读取数据块——数据块真正按需加载。
func Open(path string) (*Reader, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("asdffile: stat %q: %w", path, err)
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("asdffile: open %q: %w", path, err)
	}

	reader := &Reader{
		path:     path,
		file:     file,
		fileSize: info.Size(),
	}

	if err := reader.parseMetadata(); err != nil {
		file.Close()
		return nil, err
	}

	// 尝试建立 mmap；失败则回退到 ReadAt，不影响功能。
	mapping, err := mapFile(file, int(info.Size()))
	if err == nil && mapping != nil {
		reader.mapping = mapping
	}
	return reader, nil
}

// parseMetadata 解析魔数、Header 与索引区。
func (r *Reader) parseMetadata() error {
	const minimumSize = magicSize + lengthFieldSize + lengthFieldSize + digestSize
	if r.fileSize < minimumSize {
		return fmt.Errorf("asdffile: file too small (%d bytes)", r.fileSize)
	}

	// 魔数。
	magic := make([]byte, magicSize)
	if err := r.readAt(magic, 0); err != nil {
		return err
	}
	if !bytes.Equal(magic, Magic) {
		return fmt.Errorf("asdffile: bad magic %q (not an ASDF file)", magic)
	}

	// Header 长度与内容。
	headerLength, err := r.readUint64(magicSize)
	if err != nil {
		return err
	}
	if headerLength > uint64(r.fileSize) {
		return fmt.Errorf("asdffile: header length %d exceeds file size %d", headerLength, r.fileSize)
	}
	headerBytes := make([]byte, headerLength)
	if err := r.readAt(headerBytes, magicSize+lengthFieldSize); err != nil {
		return err
	}
	if err := yaml.Unmarshal(headerBytes, &r.header); err != nil {
		return fmt.Errorf("asdffile: parse header: %w", err)
	}

	// 索引长度与内容。
	indexOffset := magicSize + lengthFieldSize + int64(headerLength)
	indexLength, err := r.readUint64(indexOffset)
	if err != nil {
		return err
	}
	indexBytes := make([]byte, indexLength)
	if err := r.readAt(indexBytes, indexOffset+lengthFieldSize); err != nil {
		return err
	}
	entries, err := decodeIndex(indexBytes)
	if err != nil {
		return err
	}
	r.entries = entries
	r.byTopic = make(map[string]*IndexEntry, len(entries))
	for i := range entries {
		r.byTopic[entries[i].Topic] = &entries[i]
	}

	r.dataStart = indexOffset + lengthFieldSize + int64(indexLength)
	return nil
}

// readAt 从偏移处读取（不经 mmap，用于元数据解析）。
func (r *Reader) readAt(dest []byte, offset int64) error {
	read, err := r.file.ReadAt(dest, offset)
	r.bytesRead += int64(read)
	if err != nil && err != io.EOF {
		return fmt.Errorf("asdffile: read at %d: %w", offset, err)
	}
	return nil
}

// readUint64 读取一个小端 uint64。
func (r *Reader) readUint64(offset int64) (uint64, error) {
	var buffer [lengthFieldSize]byte
	if err := r.readAt(buffer[:], offset); err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint64(buffer[:]), nil
}

// Header 返回文件头。
func (r *Reader) Header() Header { return r.header }

// Topics 返回全部数据块主题（按写入顺序）。
func (r *Reader) Topics() []string {
	topics := make([]string, 0, len(r.entries))
	for _, entry := range r.entries {
		topics = append(topics, entry.Topic)
	}
	return topics
}

// Entries 返回索引条目副本。
func (r *Reader) Entries() []IndexEntry {
	out := make([]IndexEntry, len(r.entries))
	copy(out, r.entries)
	return out
}

// Entry 返回指定主题的索引条目。
func (r *Reader) Entry(topic string) (IndexEntry, bool) {
	entry, ok := r.byTopic[topic]
	if !ok {
		return IndexEntry{}, false
	}
	return *entry, true
}

// ReadBlock 按需读取单个数据块的原始字节（已解压）。
//
// 只读取该块在文件中的字节区间，不加载其他块——这是"按需加载"的实质。
func (r *Reader) ReadBlock(topic string) ([]byte, error) {
	entry, ok := r.byTopic[topic]
	if !ok {
		return nil, fmt.Errorf("asdffile: unknown topic %q", topic)
	}
	if r.closed {
		return nil, fmt.Errorf("asdffile: reader closed")
	}

	start := r.dataStart + int64(entry.Offset)
	if start+int64(entry.StoredLength) > r.fileSize {
		return nil, fmt.Errorf("asdffile: block %q out of file bounds", topic)
	}

	// 一律拷贝到调用方拥有的缓冲区，**绝不**把映射内存的切片直接返回。
	//
	// 曾经这里返回的是 mmap 区间的切片（真·零拷贝），代价是：一旦
	// Reader.Close() 解除映射，调用方手里的切片立刻变成悬空指针，
	// 再次访问会触发 SIGSEGV / "unexpected fault address"——而且崩溃
	// 发生在完全无关的调用点（实测：ArchiveStore.Restore 返回后，
	// 调用方做 bytes.Equal 时炸），几乎无法定位。
	//
	// 因此这里放弃零拷贝，换取明确的生命周期：返回值永远是调用方
	// 自己拥有的内存，与 Reader 的开关无关。mmap 的收益依然保留——
	// 省去一次 ReadAt 系统调用与内核到用户的缓冲拷贝。
	stored := make([]byte, entry.StoredLength)
	if r.mapping != nil && r.mapping.Active() {
		copy(stored, r.mapping.Bytes()[start:start+int64(entry.StoredLength)])
	} else {
		if err := r.readAt(stored, start); err != nil {
			return nil, err
		}
	}
	r.bytesRead += int64(entry.StoredLength)

	if !entry.Compressed {
		return stored, nil
	}
	raw, err := decompressBlock(stored)
	if err != nil {
		return nil, fmt.Errorf("asdffile: decompress %q: %w", topic, err)
	}
	if uint64(len(raw)) != entry.RawLength {
		return nil, fmt.Errorf("asdffile: block %q decompressed to %d bytes, index says %d",
			topic, len(raw), entry.RawLength)
	}
	return raw, nil
}

// ReadVector 读取向量块。
func (r *Reader) ReadVector(topic string) ([]float64, error) {
	entry, ok := r.byTopic[topic]
	if !ok {
		return nil, fmt.Errorf("asdffile: unknown topic %q", topic)
	}
	if entry.Kind != KindVector {
		return nil, fmt.Errorf("asdffile: topic %q is %s, not vector", topic, entry.Kind)
	}
	raw, err := r.ReadBlock(topic)
	if err != nil {
		return nil, err
	}
	return decodeFloat64Slice(raw)
}

// ReadSeries 读取时序块及其采样率。
func (r *Reader) ReadSeries(topic string) ([]float64, int, error) {
	entry, ok := r.byTopic[topic]
	if !ok {
		return nil, 0, fmt.Errorf("asdffile: unknown topic %q", topic)
	}
	if entry.Kind != KindSeries {
		return nil, 0, fmt.Errorf("asdffile: topic %q is %s, not series", topic, entry.Kind)
	}
	raw, err := r.ReadBlock(topic)
	if err != nil {
		return nil, 0, err
	}
	values, err := decodeFloat64Slice(raw)
	if err != nil {
		return nil, 0, err
	}
	return values, int(entry.Dimension), nil
}

// ReadMatrix 读取矩阵块。
func (r *Reader) ReadMatrix(topic string) (rows, cols int, values []float64, err error) {
	entry, ok := r.byTopic[topic]
	if !ok {
		return 0, 0, nil, fmt.Errorf("asdffile: unknown topic %q", topic)
	}
	if entry.Kind != KindMatrix {
		return 0, 0, nil, fmt.Errorf("asdffile: topic %q is %s, not matrix", topic, entry.Kind)
	}
	raw, err := r.ReadBlock(topic)
	if err != nil {
		return 0, 0, nil, err
	}
	values, err = decodeFloat64Slice(raw)
	if err != nil {
		return 0, 0, nil, err
	}
	if entry.Dimension == 0 {
		return 0, 0, nil, fmt.Errorf("asdffile: matrix %q has zero columns", topic)
	}
	cols = int(entry.Dimension)
	if len(values)%cols != 0 {
		return 0, 0, nil, fmt.Errorf("asdffile: matrix %q size %d not divisible by cols %d", topic, len(values), cols)
	}
	return len(values) / cols, cols, values, nil
}

// Verify 校验文件完整性：重算 SHA-256 并与尾部摘要比对。
//
// 这一步会读取整个文件（无法按需），因此只在需要完整性保证时调用。
func (r *Reader) Verify() error {
	if r.closed {
		return fmt.Errorf("asdffile: reader closed")
	}
	payloadSize := r.fileSize - digestSize
	if payloadSize <= 0 {
		return fmt.Errorf("asdffile: file too small to contain a digest")
	}

	digest := sha256.New()
	if _, err := io.Copy(digest, io.NewSectionReader(r.file, 0, payloadSize)); err != nil {
		return fmt.Errorf("asdffile: read for verification: %w", err)
	}
	stored := make([]byte, digestSize)
	if err := r.readAt(stored, payloadSize); err != nil {
		return err
	}
	if !bytes.Equal(digest.Sum(nil), stored) {
		return fmt.Errorf("asdffile: digest mismatch (file was modified or corrupted)")
	}
	return nil
}

// IsMmapActive 报告 mmap 是否真实生效。
//
// 暴露该方法是为了让调用方能确认自己确实走在零拷贝路径上，
// 而不是"以为用了 mmap、实际在 ReadAt"。
func (r *Reader) IsMmapActive() bool {
	return r.mapping != nil && r.mapping.Active()
}

// BytesRead 返回累计读取的字节数，用于验证按需加载未读整文件。
func (r *Reader) BytesRead() int64 { return r.bytesRead }

// FileSize 返回文件大小。
func (r *Reader) FileSize() int64 { return r.fileSize }

// Close 关闭文件并解除映射。
func (r *Reader) Close() error {
	if r.closed {
		return nil
	}
	r.closed = true

	var unmapErr error
	if r.mapping != nil {
		unmapErr = r.mapping.unmap()
	}
	closeErr := r.file.Close()
	if unmapErr != nil {
		return unmapErr
	}
	return closeErr
}

// ============================================================================
// float64 编解码（原生布局，小端）
// ============================================================================

// encodeFloat64Slice 把 float64 切片按 IEEE-754 小端序列化。
//
// 这是"贴近原生数组布局"的体现：8 字节定长、无分隔符、无字符串化，
// 相比 JSON 既省空间也免去解析开销。
func encodeFloat64Slice(values []float64) ([]byte, error) {
	buffer := bytes.NewBuffer(make([]byte, 0, len(values)*8))
	for _, value := range values {
		if err := binary.Write(buffer, binary.LittleEndian, value); err != nil {
			return nil, err
		}
	}
	return buffer.Bytes(), nil
}

// decodeFloat64Slice 反序列化 float64 切片。
func decodeFloat64Slice(data []byte) ([]float64, error) {
	if len(data)%8 != 0 {
		return nil, fmt.Errorf("asdffile: float64 payload size %d is not a multiple of 8", len(data))
	}
	count := len(data) / 8
	values := make([]float64, count)
	for i := 0; i < count; i++ {
		bits := binary.LittleEndian.Uint64(data[i*8:])
		values[i] = math.Float64frombits(bits)
	}
	return values, nil
}
