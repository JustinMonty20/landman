package models

import (
  "time"
  "github.com/twpayne/go-geom"
)

type Parcel struct {
  ParcelId string
  CountyId string
  Cl *changelog
}

type ParcelGeometry struct {
   ParcelId string
   Geo *geom.T
   Cl *changelog
}

type changelog struct {
  CreatedAt time.Time
  UpdatedAt time.Time
}
