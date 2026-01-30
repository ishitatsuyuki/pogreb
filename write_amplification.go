package pogreb

// WriteAmplificationEstimate holds the estimated write amplification components
// for random writes (overwrites of existing keys) in steady state.
type WriteAmplificationEstimate struct {
	// WALBytesPerWrite is the number of WAL bytes written per Put call,
	// including both direct writes and amortized compaction rewrites.
	WALBytesPerWrite float64

	// IndexBytesPerWrite is the number of index bytes written per Put call,
	// including both direct bucket updates and amortized compaction index updates.
	IndexBytesPerWrite float64

	// TotalBytesPerWrite is WALBytesPerWrite + IndexBytesPerWrite.
	TotalBytesPerWrite float64

	// WriteAmplification is the ratio of TotalBytesPerWrite to the logical
	// user data size (keySize + valueSize).
	WriteAmplification float64
}

// EstimateWriteAmplification estimates the write amplification ratio for random
// writes (overwrites of existing keys) in steady state.
//
// The estimate accounts for:
//   - WAL record overhead (2B key length + 4B value length + 4B CRC = 10 bytes)
//   - Index bucket writes (512 bytes per bucket update)
//   - Amortized compaction cost: when a WAL segment reaches the fragmentation
//     threshold f, the live fraction (1-f) is rewritten. Each promoted record
//     incurs both a WAL write and an index bucket update. In steady state,
//     each overwrite creates one dead record, and compaction promotes (1-f)/f
//     records per dead record.
//
// Compaction can occur repeatedly: a record promoted from one segment into
// another may be promoted again when that destination segment is later
// compacted. However, this does not add cost beyond the (1-f)/f factor.
// The reason is that in steady state, dead data is created at a fixed rate
// of one record per overwrite, regardless of whether the dying record was
// a direct write or a previously-promoted record. The compaction write rate
// is determined solely by the dead data creation rate and the fragmentation
// threshold:
//
//	compaction_writes = dead_data_rate * (1-f) / f
//
// Segments that contain a mix of direct and promoted records still reach
// the fragmentation threshold f at the same rate, because f is a ratio.
// The total inflow into segments is higher (direct + promoted), but the
// dead fraction within each segment is still governed by the same overwrite
// rate, so the steady-state compaction cost per overwrite is unchanged.
//
// The formula for total bytes per overwrite is:
//
//	(encodedRecordSize + bucketSize) / f
//
// where f is the compaction fragmentation threshold (default 0.5).
func EstimateWriteAmplification(keySize, valueSize int, opts *Options) WriteAmplificationEstimate {
	opts = opts.copyWithDefaults("")

	userDataSize := float64(keySize + valueSize)
	recordSize := float64(encodedRecordSize(uint32(keySize + valueSize)))
	f := float64(opts.compactionMinFragmentation)

	// Direct writes per Put call:
	//   WAL:   recordSize bytes (append encoded record)
	//   Index: bucketSize bytes (write back updated bucket)
	//
	// Amortized compaction writes per Put call:
	//   Each overwrite makes the old record dead. When a segment reaches
	//   fragmentation f, the live fraction (1-f) of the segment is rewritten.
	//   Per dead record, (1-f)/f live records are promoted. Each promoted
	//   record writes recordSize bytes to WAL and bucketSize bytes to the index.
	//
	//   Repeated compaction (promoted records being promoted again) does not
	//   change this factor. Dead data is created at one record per overwrite
	//   regardless of whether the dying record was direct or promoted, so
	//   the compaction write rate is always (1-f)/f per overwrite.
	//
	// Total = direct + compaction
	//       = (recordSize + bucketSize) + (1-f)/f * (recordSize + bucketSize)
	//       = (recordSize + bucketSize) * (1 + (1-f)/f)
	//       = (recordSize + bucketSize) / f
	walPerWrite := recordSize / f
	indexPerWrite := float64(bucketSize) / f
	total := walPerWrite + indexPerWrite

	return WriteAmplificationEstimate{
		WALBytesPerWrite:   walPerWrite,
		IndexBytesPerWrite: indexPerWrite,
		TotalBytesPerWrite: total,
		WriteAmplification: total / userDataSize,
	}
}
