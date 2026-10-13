package job

import "testing"

func TestRecordJobSPIndependentOfMastery(t *testing.T) {
	c, err := NewCharacterJob("character", "job-01")
	if err != nil {
		t.Fatal(err)
	}
	if err := c.RecordJobSP("job-01", 17); err != nil {
		t.Fatal(err)
	}
	if c.RecordedJobSP["job-01"] != 17 || c.IsMastered("job-01") {
		t.Fatal(c)
	}
	c.RecordMastery("job-01", 80, 80)
	for _, sp := range []int{120, 20, 0} {
		if err := c.RecordJobSP("job-01", sp); err != nil {
			t.Fatal(err)
		}
		if c.RecordedJobSP["job-01"] != sp || c.MasteredJobSP["job-01"] != sp || !c.IsMastered("job-01") {
			t.Fatal(c)
		}
	}
	if err := c.RecordJobSP("job-01", -1); err == nil {
		t.Fatal("negative SP accepted")
	}
}
