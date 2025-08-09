package worker

import (
	"testing"
)

func TestNewBatch(t *testing.T) {
	tests := []struct {
		name         string
		totalCount   int64
		batchSize    int8
		expectError  bool
		expectedBatch *Batch
	}{
		{
			name:        "Valid batch with exact division",
			totalCount:  100,
			batchSize:   10,
			expectError: false,
			expectedBatch: &Batch{
				BatchCount: 10,
				BatchSize:  10,
			},
		},
		{
			name:        "Valid batch with remainder",
			totalCount:  105,
			batchSize:   10,
			expectError: false,
			expectedBatch: &Batch{
				BatchCount: 11,
				BatchSize:  10,
			},
		},
		{
			name:        "Single item batch",
			totalCount:  1,
			batchSize:   1,
			expectError: false,
			expectedBatch: &Batch{
				BatchCount: 1,
				BatchSize:  1,
			},
		},
		{
			name:        "Zero total count",
			totalCount:  0,
			batchSize:   10,
			expectError: false,
			expectedBatch: &Batch{
				BatchCount: 0,
				BatchSize:  10,
			},
		},
		{
			name:        "Large batch size",
			totalCount:  5,
			batchSize:   100,
			expectError: false,
			expectedBatch: &Batch{
				BatchCount: 1,
				BatchSize:  100,
			},
		},
		{
			name:        "Negative total count",
			totalCount:  -1,
			batchSize:   10,
			expectError: true,
		},
		{
			name:        "Negative batch size",
			totalCount:  100,
			batchSize:   -1,
			expectError: true,
		},
		{
			name:        "Both negative values",
			totalCount:  -10,
			batchSize:   -5,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			batch, err := NewBatch(tt.totalCount, tt.batchSize)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if batch.BatchCount != tt.expectedBatch.BatchCount {
				t.Errorf("Expected BatchCount to be %d, got %d", tt.expectedBatch.BatchCount, batch.BatchCount)
			}

			if batch.BatchSize != tt.expectedBatch.BatchSize {
				t.Errorf("Expected BatchSize to be %d, got %d", tt.expectedBatch.BatchSize, batch.BatchSize)
			}
		})
	}
}
