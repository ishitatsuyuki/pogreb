package pogreb

// WriteAmplificationEstimate holds the estimated write amplification components
// for random writes in steady state.
type WriteAmplificationEstimate struct {
	// WALBytesPerWrite is the number of WAL bytes written per Put call,
	// including both direct writes and amortized compaction rewrites.
	WALBytesPerWrite float64

	// IndexBytesPerWrite is the number of index bytes written per Put call,
	// including both direct bucket updates, amortized compaction index updates,
	// and amortized split cost.
	IndexBytesPerWrite float64

	// TotalBytesPerWrite is WALBytesPerWrite + IndexBytesPerWrite.
	TotalBytesPerWrite float64

	// WriteAmplification is the ratio of TotalBytesPerWrite to the logical
	// user data size (keySize + valueSize).
	WriteAmplification float64
}

// EstimateWriteAmplification estimates the write amplification ratio for random
// writes in steady state.
//
// The overwriteFraction parameter (0.0 to 1.0) controls the mix of inserts
// vs overwrites. A value of 1.0 means all writes overwrite existing keys
// (steady-state table). A value of 0.0 means all writes insert new keys
// (growing table). Values in between model a mix.
//
// Overwrites incur compaction cost but no split cost:
//
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
// Inserts incur split cost but no compaction cost:
//
//   - Same WAL and index bucket write as overwrites.
//   - Amortized split cost: when the load factor exceeds 0.7, a bucket is
//     split. The split reads one bucket chain and writes two buckets (the
//     redistributed original and the new bucket). One split occurs every
//     slotsPerBucket * loadFactor ≈ 21.7 inserts.
//
// The formula for total bytes per write is:
//
//	direct + compaction + split
//
// where:
//
//	direct     = recordSize + bucketSize
//	compaction = overwriteFraction * (1-f)/f * (recordSize + bucketSize)
//	split      = (1 - overwriteFraction) * 2 * bucketSize / (slotsPerBucket * loadFactor)
func EstimateWriteAmplification(keySize, valueSize int, overwriteFraction float64, opts *Options) WriteAmplificationEstimate {
	opts = opts.copyWithDefaults("")

	userDataSize := float64(keySize + valueSize)
	recordSize := float64(encodedRecordSize(uint32(keySize + valueSize)))
	f := float64(opts.compactionMinFragmentation)

	// Direct writes per Put call:
	//   WAL:   recordSize bytes (append encoded record)
	//   Index: bucketSize bytes (write back updated bucket)
	walDirect := recordSize
	indexDirect := float64(bucketSize)

	// Amortized compaction writes per Put call (proportional to overwrite fraction):
	//   Each overwrite makes the old record dead. When a segment reaches
	//   fragmentation f, the live fraction (1-f) of the segment is rewritten.
	//   Per dead record, (1-f)/f live records are promoted. Each promoted
	//   record writes recordSize bytes to WAL and bucketSize bytes to the index.
	//
	//   Repeated compaction (promoted records being promoted again) does not
	//   change this factor. Dead data is created at one record per overwrite
	//   regardless of whether the dying record was direct or promoted, so
	//   the compaction write rate is always (1-f)/f per overwrite.
	compactionFactor := overwriteFraction * (1 - f) / f
	walCompaction := compactionFactor * recordSize
	indexCompaction := compactionFactor * float64(bucketSize)

	// Amortized split writes per Put call (proportional to insert fraction):
	//   When numKeys/(numBuckets*slotsPerBucket) > loadFactor, one bucket is
	//   split. The split writes 2 buckets: the redistributed original and the
	//   new bucket. One split occurs every slotsPerBucket * loadFactor inserts.
	//   Inserts do not create dead data, so they incur no compaction cost.
	insertFraction := 1 - overwriteFraction
	splitCost := insertFraction * 2 * float64(bucketSize) / (slotsPerBucket * loadFactor)

	walPerWrite := walDirect + walCompaction
	indexPerWrite := indexDirect + indexCompaction + splitCost
	total := walPerWrite + indexPerWrite

	return WriteAmplificationEstimate{
		WALBytesPerWrite:   walPerWrite,
		IndexBytesPerWrite: indexPerWrite,
		TotalBytesPerWrite: total,
		WriteAmplification: total / userDataSize,
	}
}
