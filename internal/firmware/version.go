// Package firmware handles Grandstream phone images: version strings, the
// local image library, and a small HTTP server phones download images from.
package firmware

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Version is a Grandstream firmware version such as 1.0.13.31.
type Version struct {
	Nums []int  `json:"nums"`
	Text string `json:"text"`
}

// "_" counts as a word character, so no \b: Release_GRP261x_1.0.13.31.zip
var versionRe = regexp.MustCompile(`(?:^|[^0-9.])(\d{1,2})\.(\d{1,3})\.(\d{1,3})\.(\d{1,4})(?:[^0-9]|$)`)

// ParseVersion pulls the first version out of s ("1.0.13.31",
// "Release_GRP261x_1.0.13.31.zip", "grp2610fw-1.0.13.31.bin").
func ParseVersion(s string) (Version, bool) {
	m := versionRe.FindStringSubmatch(s)
	if m == nil {
		return Version{}, false
	}
	v := Version{}
	for _, p := range m[1:] {
		n, _ := strconv.Atoi(p)
		v.Nums = append(v.Nums, n)
	}
	v.Text = fmt.Sprintf("%d.%d.%d.%d", v.Nums[0], v.Nums[1], v.Nums[2], v.Nums[3])
	return v, true
}

// Compare returns -1, 0 or 1.
func (v Version) Compare(o Version) int {
	for i := 0; i < len(v.Nums) || i < len(o.Nums); i++ {
		a, b := 0, 0
		if i < len(v.Nums) {
			a = v.Nums[i]
		}
		if i < len(o.Nums) {
			b = o.Nums[i]
		}
		if a != b {
			if a < b {
				return -1
			}
			return 1
		}
	}
	return 0
}

func (v Version) Newer(o Version) bool { return len(v.Nums) > 0 && v.Compare(o) > 0 }

func (v Version) IsZero() bool { return len(v.Nums) == 0 }

var familyRe = regexp.MustCompile(`(?i)^([a-z]{2,3}\d{3,4}[a-z]?)fw[._-]?`)

// FamilyFromFile reads the firmware family out of a file name:
// grp2610fw.bin -> grp2610, gxp2130fw_1.0.11.74.bin -> gxp2130.
func FamilyFromFile(name string) string {
	m := familyRe.FindStringSubmatch(strings.TrimSpace(name))
	if m == nil {
		return ""
	}
	return strings.ToLower(m[1])
}
