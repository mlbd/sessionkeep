package store

import "testing"

// A migration that has been released must never change: databases that already applied it
// record its checksum and refuse to open when the program's copy differs. This happened once
// (0006 lost a word in the rename to SessionKeep, and 0.15.1 could not open any older database).
// A new migration is added here with its checksum; an existing line is never edited.
func TestReleasedMigrationsAreUnchanged(t *testing.T) {
	released := map[int]string{
		1: "9b39a481128db60c3669170f386938a9949c56923a524becd1515ef0e737ab78",
		2: "99072d2f8d906f1c2f5700edadcf109beac84ce443fb682cadd03dddf3017137",
		3: "456b456f0fbab5e7b6594f9bf68e6747f7fe97ef78e3670b51c856a218d4df8b",
		4: "43639c36df8639b93bdb1fc82bd01ef9a6a8b2a21de3aa9e2ded0da6ce0c63d6",
		5: "7cc925ca6b1f9ba778f492a77aa0d2c383b05da429beebe8b0715d44028050b3",
		6: "552427aea76c495b12e3e91604f25e106bdaa1781d44a45ef325d50f3cadcc3d",
		7: "2b53e74029de0b954fdc96203059c03758a3e7ecb1f79d2c26d6bfca20801523",
		8: "f024be444b9ff6bca0f3254da7e502ef0e7ee9ca8d160315c539dd1c1128c9f0",
	}
	ms, err := loadMigrations(embedded, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	if len(ms) < len(released) {
		t.Fatalf("%d migrations embedded, %d were released", len(ms), len(released))
	}
	for _, m := range ms {
		want, ok := released[m.version]
		if !ok {
			t.Errorf("migration %04d_%s has no pinned checksum: add it to this test once it ships", m.version, m.name)
			continue
		}
		if m.checksum != want {
			t.Errorf("released migration %04d_%s was edited (checksum %s, released %s); restore it and put the change in a new migration", m.version, m.name, m.checksum, want)
		}
	}
}
