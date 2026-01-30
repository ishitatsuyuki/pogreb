package pogreb

import (
	"fmt"
	"testing"
)

func TestEstimateWriteAmplificationOverwrites(t *testing.T) {
	tests := []struct {
		keySize   int
		valueSize int
	}{
		{16, 100},
		{16, 1000},
		{16, 10000},
		{32, 100},
		{32, 1000},
		{32, 10000},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("key%d_value%d", tt.keySize, tt.valueSize), func(t *testing.T) {
			est := EstimateWriteAmplification(tt.keySize, tt.valueSize, 1.0, nil)

			userSize := float64(tt.keySize + tt.valueSize)
			recordSize := float64(encodedRecordSize(uint32(tt.keySize + tt.valueSize)))

			// With default compactionMinFragmentation of 0.5 and overwriteFraction=1.0:
			// total = (recordSize + bucketSize) / 0.5 = 2 * (recordSize + bucketSize)
			expectedTotal := 2 * (recordSize + float64(bucketSize))
			if est.TotalBytesPerWrite != expectedTotal {
				t.Errorf("TotalBytesPerWrite = %f, want %f", est.TotalBytesPerWrite, expectedTotal)
			}

			expectedWA := expectedTotal / userSize
			if est.WriteAmplification != expectedWA {
				t.Errorf("WriteAmplification = %f, want %f", est.WriteAmplification, expectedWA)
			}

			t.Logf("keySize=%d valueSize=%d: WA=%.2fx (WAL=%.0f B, Index=%.0f B, Total=%.0f B per %d B user write)",
				tt.keySize, tt.valueSize, est.WriteAmplification,
				est.WALBytesPerWrite, est.IndexBytesPerWrite,
				est.TotalBytesPerWrite, tt.keySize+tt.valueSize)
		})
	}
}

func TestEstimateWriteAmplificationInserts(t *testing.T) {
	tests := []struct {
		keySize   int
		valueSize int
	}{
		{16, 100},
		{16, 1000},
		{16, 10000},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("key%d_value%d", tt.keySize, tt.valueSize), func(t *testing.T) {
			est := EstimateWriteAmplification(tt.keySize, tt.valueSize, 0.0, nil)

			userSize := float64(tt.keySize + tt.valueSize)
			recordSize := float64(encodedRecordSize(uint32(tt.keySize + tt.valueSize)))

			// With overwriteFraction=0.0: no compaction, only direct + split.
			// WAL: recordSize (no compaction contribution)
			// Index: bucketSize + 2*bucketSize/(slotsPerBucket*loadFactor)
			expectedWAL := recordSize
			expectedIndex := float64(bucketSize) + 2*float64(bucketSize)/(slotsPerBucket*loadFactor)
			expectedTotal := expectedWAL + expectedIndex
			if est.TotalBytesPerWrite != expectedTotal {
				t.Errorf("TotalBytesPerWrite = %f, want %f", est.TotalBytesPerWrite, expectedTotal)
			}

			expectedWA := expectedTotal / userSize
			if est.WriteAmplification != expectedWA {
				t.Errorf("WriteAmplification = %f, want %f", est.WriteAmplification, expectedWA)
			}

			// Insert WA should be less than overwrite WA (no compaction cost).
			estOverwrite := EstimateWriteAmplification(tt.keySize, tt.valueSize, 1.0, nil)
			if est.WriteAmplification >= estOverwrite.WriteAmplification {
				t.Errorf("insert WA (%f) should be less than overwrite WA (%f)",
					est.WriteAmplification, estOverwrite.WriteAmplification)
			}

			t.Logf("keySize=%d valueSize=%d: WA=%.2fx (WAL=%.0f B, Index=%.0f B, Total=%.0f B per %d B user write)",
				tt.keySize, tt.valueSize, est.WriteAmplification,
				est.WALBytesPerWrite, est.IndexBytesPerWrite,
				est.TotalBytesPerWrite, tt.keySize+tt.valueSize)
		})
	}
}

func TestEstimateWriteAmplificationMixed(t *testing.T) {
	keySize, valueSize := 16, 100

	// WA should increase monotonically with overwrite fraction.
	var prev float64
	for _, owFrac := range []float64{0.0, 0.25, 0.5, 0.75, 1.0} {
		est := EstimateWriteAmplification(keySize, valueSize, owFrac, nil)
		if est.WriteAmplification < prev {
			t.Errorf("WA decreased from %f to %f at overwriteFraction=%f",
				prev, est.WriteAmplification, owFrac)
		}
		prev = est.WriteAmplification
		t.Logf("overwriteFraction=%.2f: WA=%.2fx (WAL=%.0f B, Index=%.0f B, Total=%.0f B)",
			owFrac, est.WriteAmplification,
			est.WALBytesPerWrite, est.IndexBytesPerWrite, est.TotalBytesPerWrite)
	}
}

func TestEstimateWriteAmplificationCustomFragmentation(t *testing.T) {
	keySize, valueSize := 16, 100

	// Test with higher fragmentation threshold (less frequent compaction).
	opts := &Options{compactionMinFragmentation: 0.7}
	est := EstimateWriteAmplification(keySize, valueSize, 1.0, opts)

	// With f=0.7, write amplification should be lower than with f=0.5
	// because compaction happens less frequently.
	estDefault := EstimateWriteAmplification(keySize, valueSize, 1.0, nil)
	if est.WriteAmplification >= estDefault.WriteAmplification {
		t.Errorf("WA with f=0.7 (%f) should be less than WA with f=0.5 (%f)",
			est.WriteAmplification, estDefault.WriteAmplification)
	}

	t.Logf("f=0.7: WA=%.2fx (Total=%.0f B per %d B user write)",
		est.WriteAmplification, est.TotalBytesPerWrite, keySize+valueSize)
	t.Logf("f=0.5: WA=%.2fx (Total=%.0f B per %d B user write)",
		estDefault.WriteAmplification, estDefault.TotalBytesPerWrite, keySize+valueSize)
}
