`pinch`
======

`pinch` is a pinch gesture detection for leap motion websocket API written in Go.

This program uses [https://github.com/whoisjake/gomotion/](https://github.com/whoisjake/gomotion/) for parsing frames into Go structs and reading frames from the default LeapMotion websocket.
<br>

The HandPinchRouter reads frames and emits Pinch object pointers which contain the point at which the pinch event occurred and the hand id that created it.<br><br>

A pinch event is sent under the following conditions:

    - there are only 2 extended fingers per hand AND
    - one or more of them disappear- which happens, according to the LeapMotion when 2 fingers converge; AND
    - the distance between them at the moment they disappear is less than a constant AND
    - the last several frames for each finger show that they are each converging on each other.

There is a demo here: [https://github.com/stuntgoat/leap-motion-pinch-gesture](https://github.com/stuntgoat/leap-motion-pinch-gesture)
Usage taken from the demo above:

	func main() {
		// from [https://github.com/whoisjake/gomotion/
		runtime.GOMAXPROCS(runtime.NumCPU())

		device, err := gomotion.GetDevice("ws://127.0.0.1:6437/v3.json")
		if err != nil {
			panic(err.Error())
		}
		defer device.Close()
		device.Listen()

	    // create a router object that will keep track and update the finger
		// positions per hand.
	    var router = pinch.HandPinchRouter{
			FrameChan: make(chan *gomotion.Frame), // send gomotion.Frame pointers here
			PinchChecks: make(map[int]pinch.HandPinchCheck), // holds fingers per hand
			PinchChan: make(chan *pinch.Pinch), // will emit Pinch pointers as the occur
		}

	    go router.RouteHand()
		for {
			select {
			case frame := <- device.Pipe:
				router.FrameChan <- frame
			case pinch := <- router.PinchChan:
				fmt.Printf("PINCH DETECTED: %+v\n", pinch)
			}
		}
	}
