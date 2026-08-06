//go:build linux

package svcmgr

import "testing"

// fakeAvail builds an idAvailability that reports the given uid/gid ids as
// taken and everything else as free.
func fakeAvail(takenUID, takenGID map[int]bool) idAvailability {
	return func(id int) (bool, bool) {
		return takenUID[id], takenGID[id]
	}
}

// TestFindAvailableSystemIDStartsAtTop verifies allocation begins at safeIDTop
// (899) and returns it when nothing is taken or reserved.
func TestFindAvailableSystemIDStartsAtTop(t *testing.T) {
	id, err := findAvailableSystemID(fakeAvail(nil, nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != safeIDTop {
		t.Fatalf("expected top of range %d, got %d", safeIDTop, id)
	}
}

// TestFindAvailableSystemIDNeverReserved verifies the allocator never returns a
// reserved id, walking every free id down the range. The AI.md PART 24 reserved
// ids (65534, 980-999, 101-110, 170-179) all sit outside the 200-899 safe range,
// so the reserved-skip is defensive; this asserts the invariant holds under a
// full scan regardless.
func TestFindAvailableSystemIDNeverReserved(t *testing.T) {
	taken := map[int]bool{}
	for top := safeIDTop; top >= safeIDBottom; top-- {
		id, err := findAvailableSystemID(fakeAvail(taken, nil))
		if err != nil {
			break
		}
		if reservedIDs[id] {
			t.Fatalf("allocator returned reserved id %d", id)
		}
		if id < safeIDBottom || id > safeIDTop {
			t.Fatalf("allocator returned out-of-range id %d", id)
		}
		taken[id] = true
	}
}

// TestFindAvailableSystemIDSkipsTakenUID verifies an id claimed only as a UID
// is skipped (UID == GID requirement means either being taken disqualifies it).
func TestFindAvailableSystemIDSkipsTakenUID(t *testing.T) {
	id, err := findAvailableSystemID(fakeAvail(map[int]bool{safeIDTop: true}, nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != safeIDTop-1 {
		t.Fatalf("expected %d after skipping taken UID %d, got %d", safeIDTop-1, safeIDTop, id)
	}
}

// TestFindAvailableSystemIDSkipsTakenGID verifies an id claimed only as a GID
// is skipped even when the UID is free.
func TestFindAvailableSystemIDSkipsTakenGID(t *testing.T) {
	id, err := findAvailableSystemID(fakeAvail(nil, map[int]bool{safeIDTop: true}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != safeIDTop-1 {
		t.Fatalf("expected %d after skipping taken GID %d, got %d", safeIDTop-1, safeIDTop, id)
	}
}

// TestFindAvailableSystemIDExhausted verifies an error when every id in the
// safe range is unavailable.
func TestFindAvailableSystemIDExhausted(t *testing.T) {
	taken := map[int]bool{}
	for id := safeIDTop; id >= safeIDBottom; id-- {
		taken[id] = true
	}
	if _, err := findAvailableSystemID(fakeAvail(taken, nil)); err == nil {
		t.Fatal("expected error when range exhausted, got nil")
	}
}

// TestFindAvailableSystemIDStaysInRange verifies the allocator never returns an
// id outside the safe range, even when only the bottom id is free.
func TestFindAvailableSystemIDStaysInRange(t *testing.T) {
	taken := map[int]bool{}
	for id := safeIDTop; id > safeIDBottom; id-- {
		taken[id] = true
	}
	id, err := findAvailableSystemID(fakeAvail(taken, nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != safeIDBottom {
		t.Fatalf("expected bottom of range %d, got %d", safeIDBottom, id)
	}
}

// TestReservedIDsMatchSpec spot-checks the reserved map against AI.md PART 24:
// nobody, the systemd/service band 980-999, and the 101-110 / 170-179 windows.
func TestReservedIDsMatchSpec(t *testing.T) {
	mustReserved := []int{65534}
	for id := 980; id <= 999; id++ {
		mustReserved = append(mustReserved, id)
	}
	for id := 101; id <= 110; id++ {
		mustReserved = append(mustReserved, id)
	}
	for id := 170; id <= 179; id++ {
		mustReserved = append(mustReserved, id)
	}
	for _, id := range mustReserved {
		if !reservedIDs[id] {
			t.Errorf("id %d must be reserved per AI.md PART 24 but is not", id)
		}
	}
	// A representative safe id must not be reserved.
	if reservedIDs[500] {
		t.Error("id 500 should not be reserved")
	}
}
