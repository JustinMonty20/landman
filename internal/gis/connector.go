package gis

import (
  "context"
  "github.com/JustinMonty20/landman/internal/models"
)
// first iteration here. 
// thinking that each county connector that implements this will end up having the timeing frequency to update. 
// each county will have its diff logic and then transform the information// into our simplified type.
type Connector interface {
  GetData(ctx context.Context) ([]models.Parcel, error)
  Diff(ctx context.Context) ([]models.Parcel, error)
}
