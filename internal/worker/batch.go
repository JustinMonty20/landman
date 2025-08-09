package worker

import (
  "fmt"
)

type Batch struct {
  // number of batches of work to do.
  BatchCount int64
  // numbber of items in each batch.
  BatchSize int8
}

func NewBatch(totalCount int64, batchSize int8) (*Batch, error) {
  if totalCount < 0 {
    return nil, fmt.Errorf("totlCount must be greater than 0"); 
  } 

  if batchSize < 0 {
    return nil, fmt.Errorf("batchSize must be greater than 0");  
  }

  batchCount := (totalCount + int64(batchSize) - 1) / int64(batchSize); 
  
  return &Batch {
    BatchCount: batchCount,
    BatchSize: batchSize,
  }, nil
}
