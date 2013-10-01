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
