package pinch

import (
	"fmt"
	"log"
	"math"
	"os"
	"time"

	"github.com/stuntgoat/circbuf"
	"github.com/whoisjake/gomotion"
)


const (
	// threshold between 2 pointables to register a pinch
	PINCH_DISTANCE_THRESHOLD = 1900

	// number of previous pointable objects to store in a circular buffer
	// for each finger per hand.
	MAX_POINTABLES_PER_HISTORY = 15

	// number of times that the last distance between 2 points
	// can be greater than the current distance, when checking convergence.
	CONVERGENCE_THRESHOLD = 6

	DEBUG = false
	DEBUG_COLLECT = false
	DEBUG_DISTANCE = false
	DEBUG_CONVERGENCE = false
	DEBUG_PINCH = false
)

var logger = log.New(os.Stderr, "[leap pinch] ", log.LstdFlags|log.Lshortfile)

func Debug(msg string) {
	if DEBUG {
		logger.Println(msg)
	}
}


// MyPointable holds a circular buffer of Pointable objects
type MyPointable struct {
	History *circbuf.Circ
	lastUpdate time.Time
}

// calculateConvergence takes a MyPointable object and calculates
// the difference between the last number of respective 
// pointable object coordinates.
func (mp *MyPointable) CalculateConvergence(p *MyPointable) bool {
	// get last point data to confirm if the points converging.
	var myA interface{}
	var myB interface{}

	var dCurrent float64
	var dLast float64

	var failThreshold int

	var p1Len = mp.History.Added
	var p2Len = p.History.Added

	var max int64
	var maxCount int

	if p1Len == p2Len || p1Len < p2Len {
		maxCount = int(p1Len)
	} else {
		maxCount = int(p2Len)
	}

	if max > MAX_POINTABLES_PER_HISTORY {
		maxCount = MAX_POINTABLES_PER_HISTORY
	}

	for i := 0; i < maxCount; i += 2 { // skip every other frame
		myA, _ = mp.History.ReadFromStart(i)
		myAp, okA := myA.(gomotion.Pointable)

		myB, _ = p.History.ReadFromStart(i)
		myBp, okB := myB.(gomotion.Pointable)

		if okA && okB {
			dCurrent = DistanceBetweenPointables(&myAp, &myBp)
		} else {
			logger.Fatal("fail to cast interface from circular buffer to a Pointable")
		}

		if dLast < dCurrent && dLast != 0 {
			failThreshold++
		}
		dLast = dCurrent
	}

	if failThreshold > CONVERGENCE_THRESHOLD {
		if DEBUG_CONVERGENCE {
			Debug(fmt.Sprintf("failThreshold: %d", failThreshold))
		}
		return false
	}
	return true
}


// HandPinchCheck is an object that represents a hand.
// hands can change ids if they disappear and come back into the LeapMotion's
// view. A Hand can have 1 - 5 Pointables. We keep track the last 15 frames
// seen for each pointable. Pointable ids can/will change as pointables
// enter and leave the LeapMotion's view.
type HandPinchCheck struct {
	// unique per hand in each frame
	HandId int

	// listens for pointables
	PointableChan chan gomotion.Pointable

	// sends a pinch event to the listener
	PinchChan *chan *Pinch

	// pointable id to last update time
	LastUpdate map[int]time.Time

	// listen if a finger disappeared
	FingerDisappeared chan bool

	// list of pointables per hand and their history
	Pointables map[int]*MyPointable
}

func (hPinchCheck *HandPinchCheck) ListenForPointables() {

	var pair []*MyPointable
	var myP *MyPointable
	var ok bool
	var converging bool
	var args []*gomotion.Pointable

	for {
		select {
		case p := <- hPinchCheck.PointableChan:
			hPinchCheck.LastUpdate[p.Id] = time.Now()

			// create this MyPointable object if it doesn't exist
			myP, ok = hPinchCheck.Pointables[p.Id]
			if ok == false {
				cb := circbuf.NewCircBuf(MAX_POINTABLES_PER_HISTORY)
				myP = &MyPointable{
					History: cb,
				}
			}
			hPinchCheck.Pointables[p.Id] = myP
			myP.History.AddItem(p)
			myP.lastUpdate = time.Now()

		case <- hPinchCheck.FingerDisappeared:
			// Check if we have only 2 pointables that have been seen
			// recently.

			pair = make([]*MyPointable, 0)
			for _, pntbl := range hPinchCheck.Pointables {
				// Check the number of pointables that are new
				isNew := bool(time.Since(pntbl.lastUpdate) < time.Duration(50 * time.Millisecond))
				// at least a few frames in history?
				if isNew && (pntbl.History.Added >= 6) {
					pair = append(pair, pntbl)
				}
				if DEBUG_COLLECT {
					msg := fmt.Sprintf("pointable is new: %t", isNew)
					Debug(msg)
					msg = fmt.Sprintf("pointable had added %d frames", pntbl.History.Added)
					Debug(msg)
				}
			}

			// two fingers might mean a pinch occured
			if len(pair) == 2 {
				// extract the Pointable from the circular buffer
				var lastItem interface{}
				var err error
				args = make([]*gomotion.Pointable, 0)

				for _, pntbl := range pair {

					lastItem, err = pntbl.History.ReadFromEnd(0)
					if err != nil {
						logger.Fatal(err.Error)
					}
					goP, ok := lastItem.(gomotion.Pointable)
					if ok {
						args = append(args, &goP)
					}
				}

				dist := DistanceBetweenPointables(args[0], args[1])
				if dist < PINCH_DISTANCE_THRESHOLD {

					converging = pair[0].CalculateConvergence(pair[1])
					if converging {
						Debug("sending pinch on pinch channel")

						pinch := new(Pinch)
						pinch.SetFrom2Pointables(args[0], args[1])
						*hPinchCheck.PinchChan <- pinch
						goto REMOVE_OLD
					}
					if DEBUG_CONVERGENCE {
						Debug("failed to converge")
					}

				} else {
					if DEBUG_DISTANCE {
						logger.Printf("PINCH_DISTANCE_THRESHOLD: %d\tdistance: %f\n", PINCH_DISTANCE_THRESHOLD, dist)
					}
				}
			} else  {
				if DEBUG_COLLECT {
					logger.Printf("could not find 2 pointables. Found %d", len(pair))
				}
			}
			goto REMOVE_OLD
		}

	REMOVE_OLD:
		for id, mpt := range hPinchCheck.Pointables {
			if time.Since(mpt.lastUpdate) > time.Duration(60 * time.Millisecond) {
				delete(hPinchCheck.Pointables, id)
			}
		}
	}
}


// HandPinchCheck reads LeapMotion frames sends Pinch objects shortly
// after they occur.
type HandPinchRouter struct {
	// listens for frames
	FrameChan chan *gomotion.Frame

	// map of hand ids to objects that calculate Pinch events for that hand.
	PinchChecks map[int]HandPinchCheck

	// emits Pinch object pointers
	PinchChan chan *Pinch
}

// PPH stands for Pointables Per Hand; used for counting pointables
// per hand within the HandPinchRouter
type PPH struct {
	HandId int
	NumPointables int
}

// RouteHands will route a Pointable to the respective hand pinch
// channel.
func (hPinchRouter *HandPinchRouter) RouteHand() {
	var currPerHand = map[int]*PPH{}
	var pastPerHand = map[int]*PPH{}
	var handId int
	for frame := range hPinchRouter.FrameChan {
		for _, pointable := range frame.Pointables {
			handId = pointable.HandId

			// sometimes we receive a frame with an unknown hand id
			if handId == -1 {
				continue
			}

			// check the current hand id pointables; 'pointables per hand'.
			// If one is not found, we create one
			if pph, ok := currPerHand[handId]; ok {
				pph.NumPointables += 1
			} else {
				pph = &PPH{
					HandId: handId,
					NumPointables: 0,
				}
				pph.NumPointables++
				currPerHand[handId] = pph
			}

			pc, ok := hPinchRouter.PinchChecks[handId];
			if ok {
				pc.PointableChan <- pointable
			} else {
				hpc := HandPinchCheck{
					HandId: pointable.HandId,
					PointableChan: make(chan gomotion.Pointable),
					LastUpdate: make(map[int]time.Time),
					Pointables: make(map[int]*MyPointable),
					FingerDisappeared: make(chan bool),
					PinchChan: &hPinchRouter.PinchChan,
				}
				go hpc.ListenForPointables()
				hpc.PointableChan <- pointable
				hPinchRouter.PinchChecks[handId] = hpc
			}
		}
		// calculate the current pointables per hand and see if any disappeared,
		// if so, send the FingerDisappeared to the hand's FingerDisappeared chan.
		for pHandId, pph := range pastPerHand {
			if cHnum, ok := currPerHand[pHandId]; ok {
				if cHnum.NumPointables < pph.NumPointables {
					hPinchRouter.PinchChecks[pHandId].FingerDisappeared <- true;
				}
			}
		}
		pastPerHand = currPerHand
		currPerHand = map[int]*PPH{}
	}
}

// Pinch represents a pinch event in 3 dimensional space for a particular hand.
type Pinch struct {
	X float64
	Y float64
	Z float64
	HandId int
}

// SetFrom2Pointables sets the fields in a Pinch object from 2 gomotion.Pointables.
func (p *Pinch) SetFrom2Pointables(p1, p2 *gomotion.Pointable) {
	p1X, p1Y, p1Z := XYZFromPointable(p1)
	p2X, p2Y, p2Z := XYZFromPointable(p2)

	p.X = Halfway(p1X, p2X)
	p.Y = Halfway(p1Y, p2Y)
	p.Z = Halfway(p1Z, p2Z)
	p.HandId = p1.HandId
}


func XYZFromPointable(p *gomotion.Pointable) (x, y, z float64) {
	return float64(p.TipPosition[0]), float64(p.TipPosition[1]), float64(p.TipPosition[2])
}

func DistanceBetweenPointables(pointable1, pointable2 *gomotion.Pointable) float64 {
	p1X, p1Y, p1Z := XYZFromPointable(pointable1)
	p2X, p2Y, p2Z := XYZFromPointable(pointable2)

	return DistanceBetween(p1X, p1Y, p1Z, p2X, p2Y, p2Z)
}

// Halfway returns the coordinate between 2 coordinates in the same dimension.
func Halfway(a, b float64) float64 {
	return float64(a + (0.5 * (b - a)))
}

// DistanceBetween takes the distance between 2 3D points and
// caluclates the total distance between them by calculating the
// sum of the squares of the difference of each x, y and z coordinate.
func DistanceBetween(x, y, z, x2, y2, z2 float64) float64 {
	xSqDiff := math.Pow(x - x2, 2)
	ySqDiff := math.Pow(y - y2, 2)
	zSqDiff := math.Pow(z - z2, 2)
	return xSqDiff + ySqDiff + zSqDiff
}
