// Package stats turns the volume streamed through RAM into the interface
// metric "storage resource saved": a byte count plus a wear comparison across
// HDD, SSD and M.2 drive classes (back_spec.md §3.9).
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

// Wear is the wear calculation for one drive class.
type Wear struct {
	Class       DeviceClass `json:"class"`
	Label       string      `json:"label"`
	Bytes       int64       `json:"bytes"`
	Percent     float64     `json:"percent"`
	Approximate bool        `json:"approximate"`
}

// WearReport summarises all classes and their average.
type WearReport struct {
	Classes []Wear  `json:"classes"`
	Average float64 `json:"average"`
}

// Calculate builds the report for a number of bytes.
//
// Volume is converted to GiB (1024^3) — the same binary count the client
// uses for "5.78 ГБ"-style formatting, so both sides always display the same
// number.
func Calculate(savedBytes int64) WearReport {
	gb := float64(savedBytes) / (1024 * 1024 * 1024)
	classes := []Wear{
		{Class: ClassHDD, Label: "HDD", Bytes: savedBytes, Percent: gb * hddWearPerGB, Approximate: true},
		{Class: ClassSSD, Label: "SSD", Bytes: savedBytes, Percent: gb * ssdWearPerGB},
		{Class: ClassM2, Label: "M.2 / NVMe", Bytes: savedBytes, Percent: gb * m2WearPerGB},
	}
	sum := 0.0
	for _, c := range classes {
		sum += c.Percent
	}
	return WearReport{Classes: classes, Average: sum / float64(len(classes))}
}
