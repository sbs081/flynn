package zfs

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	. "github.com/flynn/flynn/Godeps/_workspace/src/github.com/flynn/go-check"
	//	gzfs "github.com/flynn/flynn/Godeps/_workspace/src/github.com/mistifyio/go-zfs"
	"github.com/flynn/flynn/pkg/testutils"
)

type ZfsTransmitTests struct {
	pool1 TempZpool
	pool2 TempZpool
}

var _ = Suite(&ZfsTransmitTests{})

func (ZfsTransmitTests) SetUpSuite(c *C) {
	// Skip all tests in this suite if not running as root.
	// Many zfs operations require root priviledges.
	testutils.SkipIfNotRoot(c)
}

func (s *ZfsTransmitTests) SetUpTest(c *C) {
	s.pool1.SetUpTest(c)
	s.pool2.SetUpTest(c)
}

func (s *ZfsTransmitTests) TearDownTest(c *C) {
	s.pool1.TearDownTest(c)
	s.pool2.TearDownTest(c)
}

/*
	Testing behaviors of 'zfs send' & 'zfs recv' in isolation to make sure deltas work the way we expect.

	See integration tests for taking the full trip over the wire through the REST API.
*/
func (s *ZfsTransmitTests) TestZfsSendRecvFull(c *C) {
	// create volume; add content; snapshot it.
	// note that 'zfs send' refuses anything but snapshots.
	v, err := s.pool1.VolProv.NewVolume()
	c.Assert(err, IsNil)
	f, err := os.Create(filepath.Join(v.Location(), "alpha"))
	c.Assert(err, IsNil)
	f.Close()
	snap, err := s.pool1.VolProv.CreateSnapshot(v)
	c.Assert(err, IsNil)

	var buf bytes.Buffer
	s.pool1.VolProv.SendSnapshot(snap, &buf)

	// send stream to this pool; should get a new snapshot volume
	snapRestored, err := s.pool1.VolProv.ReceiveSnapshot(bytes.NewBuffer(buf.Bytes()))
	c.Assert(err, IsNil)
	c.Assert(snapRestored.IsSnapshot(), Equals, true)
	// check that contents came across
	c.Assert(snapRestored.Location(), testutils.DirContains, []string{"alpha"})

	// send stream to another pool; should get a new volume
	snapRestored, err = s.pool2.VolProv.ReceiveSnapshot(bytes.NewBuffer(buf.Bytes()))
	c.Assert(err, IsNil)
	c.Assert(snapRestored.IsSnapshot(), Equals, true)
	// check that contents came across
	c.Assert(snapRestored.Location(), testutils.DirContains, []string{"alpha"})
}

/*
	Test that sending incremental deltas works (and is smaller than wholes).
*/
func (s *ZfsTransmitTests) TestZfsSendRecvIncremental(c *C) {
	// create volume; add content; snapshot it.
	v, err := s.pool1.VolProv.NewVolume()
	c.Assert(err, IsNil)
	f, err := os.Create(filepath.Join(v.Location(), "alpha"))
	c.Assert(err, IsNil)
	f.Close()
	snap, err := s.pool1.VolProv.CreateSnapshot(v)
	c.Assert(err, IsNil)

	var buf bytes.Buffer
	s.pool1.VolProv.SendSnapshot(snap, &buf)
	fmt.Printf("note: size of snapshot stream is %d bytes\n", buf.Len()) // 41680

	// send stream to another pool; should get a new snapshot volume
	snapRestored, err := s.pool1.VolProv.ReceiveSnapshot(bytes.NewBuffer(buf.Bytes()))
	c.Assert(err, IsNil)
	c.Assert(snapRestored.IsSnapshot(), Equals, true)
	// check that contents came across
	c.Assert(snapRestored.Location(), testutils.DirContains, []string{"alpha"})

	// edit files; make another snapshot
	f, err = os.Create(filepath.Join(v.Location(), "beta"))
	c.Assert(err, IsNil)
	f.Close()
	snap2, err := s.pool1.VolProv.CreateSnapshot(v)
	c.Assert(err, IsNil)

	// make another complete snapshot, just to check size
	buf.Reset()
	s.pool1.VolProv.SendSnapshot(snap2, &buf)
	fmt.Printf("note: size of bigger snapshot stream is %d bytes\n", buf.Len()) // 42160

	// FIXME we're gonna wanna rewrap this
	output, err := exec.Command("zfs", "send", "-i", snap.(*zfsVolume).dataset.Name, snap2.(*zfsVolume).dataset.Name).Output()
	c.Assert(err, IsNil)
	fmt.Printf("note: size of incremental stream is %d bytes\n", len(output)) // 10064

	// send that incremental stream in to the second pool
	// FIXME : this restore sig works... but the dataset created right now doesn't know enough about data sharing
	snapRestored, err = s.pool1.VolProv.ReceiveSnapshot(bytes.NewBuffer(output))
	c.Assert(err, IsNil)
	c.Assert(snapRestored.IsSnapshot(), Equals, true)
	// check that contents came across
	c.Assert(snapRestored.Location(), testutils.DirContains, []string{"alpha"})
	c.Assert(snapRestored.Location(), testutils.DirContains, []string{"beta"})
}

/*
	Test that sending an incremental delta to a dataset that doesn't have
	enough context errors visibly.
*/
func (s *ZfsTransmitTests) TestZfsSendRecvIncrementalFubar(c *C) {
	// TODO assert we have reasonable detection of invalid situations
}
