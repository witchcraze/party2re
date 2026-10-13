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

func TestRecordedJobPresenceDoesNotFollowSavedPoints(t *testing.T) {
	c := CharacterJob{CharacterID: "character", CurrentJobID: "current", RecordedJobSP: map[string]int{"current": 111, "previous": 0}, MasteredJobSP: map[string]int{"points-only": 80}}
	for _, id := range []string{"current", "previous", "points-only", "unknown", ""} {
		if c.HasRecordedJob(id) {
			t.Fatalf("SP/identity created a job record: %s", id)
		}
	}
	c.History = []Change{{FromJobID: "previous", ToJobID: "current"}, {FromJobID: "missing-sp", ToJobID: "current"}}
	if !c.HasRecordedJob("previous") || !c.HasRecordedJob("missing-sp") || c.HasRecordedJob("current") {
		t.Fatal(c)
	}
	c.MasteredJobs = []string{"points-only", "master-without-sp"}
	if !c.HasRecordedJob("points-only") || !c.HasRecordedJob("master-without-sp") {
		t.Fatal(c)
	}
	// Same-job reselection records SP without a cross-job transition.
	if err := c.RecordJobSP("current", 0); err != nil {
		t.Fatal(err)
	}
	if !c.HasRecordedJob("current") {
		t.Fatal("explicit job record lost", c)
	}
}
