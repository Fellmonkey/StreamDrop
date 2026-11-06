// Package stats turns the volume streamed through RAM into the interface
// metric "storage resource saved": a byte count plus a wear comparison across
// HDD, SSD and M.2 drive classes (back_spec.md §3.9).
//
// The same volume is also converted to "time saved": the classic alternative
// would download to disk first and then re-upload from disk, so the saved time
// is the round-trip (write + read) through each drive class at average speeds.
package stats

// DeviceClass is a drive class.
type DeviceClass string

const (
	ClassHDD DeviceClass = "hdd"
	ClassSSD DeviceClass = "ssd"
	ClassM2  DeviceClass = "m2"
)

// wearPerGB is the percentage of drive endurance consumed by writing 1 GiB.
//
// SSD: TBW ≈ 600 per 1 TB  ->  1 GB ≈ 0.0001667 % of endurance
// M.2: TBW ≈ 1200 per 1 TB ->  1 GB ≈ 0.0000833 %
// HDD: has no finite write endurance; the model reflects mechanical wear
//
//	(≈ 0.000025 % per GB) and is flagged as approximate.
const (
	ssdWearPerGB = 0.0001667
	m2WearPerGB  = 0.0000833
	hddWearPerGB = 0.0000250
)

// Average sequential speeds per drive class, in bytes per second. These are
// typical off-the-shelf numbers used only to estimate the round-trip time a
// download-to-disk-then-upload flow would have taken.
const (
	hddReadBps  = 180 * 1000 * 1000
	hddWriteBps = 150 * 1000 * 1000
	ssdReadBps  = 520 * 1000 * 1000
	ssdWriteBps = 450 * 1000 * 1000
	m2ReadBps   = 3200 * 1000 * 1000
	m2WriteBps  = 2800 * 1000 * 1000
)

// DefaultNetDownBps / DefaultNetUpBps are the modeled internet speeds: 20 MB/s
// both ways. The model is intentionally not capped by a tariff — the point is
// to show that the disk round-trip (write + read) is the bottleneck, so the
// saved time reflects the drive speeds.
const (
	DefaultNetDownBps int64 = 20 * 1000 * 1000
	DefaultNetUpBps   int64 = 20 * 1000 * 1000
)

// Rewrite capacity per 1 TB of drive, in TB (TBW). HDDs are not rated in TBW
// (mechanical wear, no meaningful finite rewrite quota), so HDD is nil.
const (
	ssdRewriteCapacityTb = 600.0
	m2RewriteCapacityTb  = 1200.0
)

// Wear is the wear calculation for one drive class.
type Wear struct {
	Class       DeviceClass `json:"class"`
	Label       string      `json:"label"`
	Bytes       int64       `json:"bytes"`
	Percent     float64     `json:"percent"`
	Approximate bool        `json:"approximate"`

	// TimeSavedSeconds is how long the classic download-to-disk-then-upload
	// path would have spent writing and reading this volume on this class.
	TimeSavedSeconds float64 `json:"time_saved_seconds"`
	// ReadSpeedBps / WriteSpeedBps are the model speeds used for the estimate.
	ReadSpeedBps  int64 `json:"read_speed_bps"`
	WriteSpeedBps int64 `json:"write_speed_bps"`
	// RewriteCapacityTb is the endurance (TBW per 1 TB drive); nil for HDD.
	RewriteCapacityTb *float64 `json:"rewrite_capacity_tb"`
}

// WearReport summarises all classes and their average.
type WearReport struct {
	Classes []Wear  `json:"classes"`
	Average float64 `json:"average"`
	// AverageTimeSavedSeconds is the mean round-trip time across classes.
	AverageTimeSavedSeconds float64 `json:"average_time_saved_seconds"`
	// NetDownBps / NetUpBps are the internet speeds used in the model
	// (measured averages when available, defaults otherwise).
	NetDownBps int64 `json:"net_down_bps"`
	NetUpBps   int64 `json:"net_up_bps"`
}

// Calculate builds the report for a number of bytes using the default modeled
// internet speeds.
func Calculate(savedBytes int64) WearReport {
	return CalculateWithSpeeds(savedBytes, DefaultNetDownBps, DefaultNetUpBps)
}

// CalculateWithSpeeds builds the report with explicit internet speeds. These
// are informational: the classic flow being compared is download-to-disk then
// upload-from-disk, so the saved time is the disk round-trip (write + read) at
// the drive's own average speeds. The internet speeds are shown in the UI so
// the user sees they are not the bottleneck.
//
// Volume is converted to GiB (1024^3) — the same binary count the client
// uses for "5.78 ГБ"-style formatting, so both sides always display the same
// number.
func CalculateWithSpeeds(savedBytes int64, netDownBps, netUpBps int64) WearReport {
	if netDownBps <= 0 {
		netDownBps = DefaultNetDownBps
	}
	if netUpBps <= 0 {
		netUpBps = DefaultNetUpBps
	}
	gb := float64(savedBytes) / (1024 * 1024 * 1024)
	classes := []Wear{
		{
			Class: ClassHDD, Label: "HDD", Bytes: savedBytes,
			Percent: gb * hddWearPerGB, Approximate: true,
			TimeSavedSeconds: roundTripSeconds(savedBytes, hddWriteBps, hddReadBps),
			ReadSpeedBps:     hddReadBps, WriteSpeedBps: hddWriteBps,
			RewriteCapacityTb: nil,
		},
		{
			Class: ClassSSD, Label: "SSD", Bytes: savedBytes,
			Percent:          gb * ssdWearPerGB,
			TimeSavedSeconds: roundTripSeconds(savedBytes, ssdWriteBps, ssdReadBps),
			ReadSpeedBps:     ssdReadBps, WriteSpeedBps: ssdWriteBps,
			RewriteCapacityTb: float64Ptr(ssdRewriteCapacityTb),
		},
		{
			Class: ClassM2, Label: "M.2 / NVMe", Bytes: savedBytes,
			Percent:          gb * m2WearPerGB,
			TimeSavedSeconds: roundTripSeconds(savedBytes, m2WriteBps, m2ReadBps),
			ReadSpeedBps:     m2ReadBps, WriteSpeedBps: m2WriteBps,
			RewriteCapacityTb: float64Ptr(m2RewriteCapacityTb),
		},
	}
	sum := 0.0
	sumTime := 0.0
	for _, c := range classes {
		sum += c.Percent
		sumTime += c.TimeSavedSeconds
	}
	return WearReport{
		Classes:                 classes,
		Average:                 sum / float64(len(classes)),
		AverageTimeSavedSeconds: sumTime / float64(len(classes)),
		NetDownBps:              netDownBps,
		NetUpBps:                netUpBps,
	}
}

// roundTripSeconds is the time to write the volume to disk at the drive's
// write speed and read it back at the drive's read speed.
func roundTripSeconds(bytes int64, writeBps, readBps int64) float64 {
	if bytes <= 0 || writeBps <= 0 || readBps <= 0 {
		return 0
	}
	return float64(bytes)/float64(writeBps) + float64(bytes)/float64(readBps)
}

func float64Ptr(v float64) *float64 { return &v }
