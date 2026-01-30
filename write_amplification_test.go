package pogreb

import (
	"fmt"
	"testing"
)

func TestEstimateWriteAmplification(t *testing.T) {
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
			est := EstimateWriteAmplification(tt.keySize, tt.valueSize, nil)

			userSize := float64(tt.keySize + tt.valueSize)
			recordSize := float64(encodedRecordSize(uint32(tt.keySize + tt.valueSize)))

			// With default compactionMinFragmentation of 0.5:
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

func TestEstimateWriteAmplificationCustomFragmentation(t *testing.T) {
	keySize, valueSize := 16, 100

	// Test with higher fragmentation threshold (less frequent compaction).
	opts := &Options{compactionMinFragmentation: 0.7}
	est := EstimateWriteAmplification(keySize, valueSize, opts)

	// With f=0.7, write amplification should be lower than with f=0.5
	// because compaction happens less frequently.
	estDefault := EstimateWriteAmplification(keySize, valueSize, nil)
	if est.WriteAmplification >= estDefault.WriteAmplification {
		t.Errorf("WA with f=0.7 (%f) should be less than WA with f=0.5 (%f)",
			est.WriteAmplification, estDefault.WriteAmplification)
	}

	t.Logf("f=0.7: WA=%.2fx (Total=%.0f B per %d B user write)",
		est.WriteAmplification, est.TotalBytesPerWrite, keySize+valueSize)
	t.Logf("f=0.5: WA=%.2fx (Total=%.0f B per %d B user write)",
		estDefault.WriteAmplification, estDefault.TotalBytesPerWrite, keySize+valueSize)
}
