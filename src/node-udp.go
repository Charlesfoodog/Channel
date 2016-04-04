package main

/*
  UBC CS416 Distributed Systems Project Source Code

  @author: Abrar, Ito, Mimi, Shariq
  @date: Mar. 1 2016 - Apr. 11 2016.

  Usage:
    go run node.go [node ip:port] [starter-node ip:port] [-r=replicationFactor] [-t]

    [node ip:port] : this node's ip/port combo
    [starter-node ip:port] : the entry point node's ip/port combo
    [-r=replicationFactor] : replication factor for Keys, default r = 2
    [-t] : trace mode for debugging

  Copy/paste for quick testing:
    "go run node.go :0 :6666" <-- trace off, default replication factor (2)
    "go run node.go :0 :6666 -t -r=5" <-- trace on, r = 5
*/

import (
  "crypto/sha1"
  "encoding/hex"
  "encoding/json"
  "flag"
  "fmt"
  "net"
  "os"
  "time"
  "sync"
  "math"
  "strconv"
  "math/big"
  "runtime"
  //"transfile"
)

// =======================================================================
// ====================== Global variables/types =========================
// =======================================================================
// Some types used for RPC communication.
type nodeRPCService int
type NodeMessage struct {
  Msg string
}
type ConnResponse struct {
  Conn *net.TCPConn
}
type CommandMessage struct {
  Cmd string
  SourceAddr string
  DestAddr string
  Key string
  Val string
}

// Some static command line options.
var traceMode bool
var replicationFactor int
var store map[string]string
var ftab map[int64]string
var successor int64
var successorAddr string
var predecessor int64
var predecessorAddr string
var identifier int64
var m float64
var c chan string
var myAddr string

var successorAliveChannel chan bool
var predecessorAliveChannel chan bool
//var timeout chan bool

// The finger table.
// Reading? Wrap RLock/RUnlock: lock.RLock(), lock.fingerTable[..], lock.RUnlock()
// Writing? Wrap Lock/Unlock: lock.Lock(), lock.fingerTable[..] = .. ,lock.Unlock()
var lock = struct {
  sync.RWMutex
  fingerTable map[string]string
}{fingerTable: make(map[string]string)} // initialize it here (or in main)

// =======================================================================
// ======================= Function definitions ==========================
// =======================================================================
/* Updates or inserts a finger table entry at this node. Called by other nodes.
 */
func (this *nodeRPCService) UpdateFingerTableEntry(id string, addr string,
  reply *NodeMessage) error {
  // Lock 'em up.
  lock.Lock()
  defer lock.Unlock()

  // Update.
  lock.fingerTable[id] = addr

  // Let the other guy know it went well.
  reply.Msg = "Ok"
  return nil
}

/* Grabs a finger table entry at this node. Called by other nodes.
 */
func (this *nodeRPCService) GetFingerTableEntry(id string,
  reply *NodeMessage) error {
  // Lock 'em up (read mode).
  lock.RLock()
  defer lock.RUnlock()

  // Send the Value of the entry that some guy requested.
  reply.Msg = lock.fingerTable[id]
  return nil
}

/* Removes a finger table entry at this node. Called by other nodes.
 */
func (this *nodeRPCService) DeleteFingerTableEntry(id string,
  reply *NodeMessage) error {
  // Lock 'em up.
  lock.Lock()
  defer lock.Unlock()

  // Update.
  delete(lock.fingerTable, id)

  // Let the other guy know it went well.
  reply.Msg = "Ok"
  return nil
}

/* Checks error Value and prints/exits if non nil.
 */
func checkError(err error) {
  if err != nil {
    fmt.Println("Error string: ", err)
    os.Exit(-1)
  }
}

/* Returns the SHA1 hash Value as a string, of a Key k.
 */
func computeSHA1Hash(Key string) string {
  buf := []byte(Key)
  h := sha1.New()
  h.Write(buf)
  str := hex.EncodeToString(h.Sum(nil))
  return str
}


/* Send periodic heartbeats to let predecessor know this node is still alive.
 */
func sendAliveMessage(addr string) { 
      msg := CommandMessage{"_heartbeat", myAddr, addr, strconv.FormatInt(identifier, 10), myAddr}
      aliveMessage, err := json.Marshal(msg)
      checkError(err)
      b := []byte(aliveMessage)
      sendMessage(addr, b)
}

func askIfAlive(timeout chan bool, addr string) {
    msg := CommandMessage{"_alive?", myAddr, addr, strconv.FormatInt(identifier, 10), myAddr}
    aliveMessage, err := json.Marshal(msg)
    checkError(err)
    b := []byte(aliveMessage)
    sendMessage(addr, b)
    time.Sleep(5 * time.Second)
    timeout <- true
}

/* Handle heartbeats
 */
// func handleHeartbeats() {

//   for {
//     if successorAddr != "" && predecessorAddr != "" {
//       timeout_s := make(chan bool, 1)
//       timeout_p := make(chan bool, 1)
//       go askIfAlive(timeout_s, successorAddr)
//       go askIfAlive(timeout_p, predecessorAddr)

//       select {
//       case <-successorAliveChannel:
//         fmt.Println("Heartbeat from successor: ", successorAddr)
//       case <-predecessorAliveChannel:
//         fmt.Println("Heartbeat from predecessor", predecessorAddr)
//       case <-timeout_s:
//         // timed out, my successor might be dead. time to make some changes in our secret circle
//         fmt.Println("Timed out on successor heartbeat")
//         // locate new successor if any
//         // update predecessor of new 
//         successor = -1
//         successorAddr = ""
//         stabilizeNode("successor")
//       case <-timeout_p:
//         fmt.Println("Timed out on predecessor heartbeat")
//         predecessor = -1
//         predecessorAddr = ""
//         //stabilizeNode()
//       }     
//       time.Sleep(5 * time.Second)
//     } else {
//         //fmt.Println("Looping until we have both successor and predecessor")
//     }
//   }
// }

func handlePredecessorHeartbeats() {

  for {
    if predecessorAddr != "" {
      timeout_p := make(chan bool, 1)
      go askIfAlive(timeout_p, predecessorAddr)

      select {
      case <-predecessorAliveChannel:
        fmt.Println("Heartbeat from predecessor", predecessorAddr)
      case <-timeout_p:
        fmt.Println("Timed out on predecessor heartbeat")
        predecessor = -1
        predecessorAddr = ""
        //stabilizeNode()
      }     
      time.Sleep(5 * time.Second)
    } else {
        //fmt.Println("Looping until we have both successor and predecessor")
      runtime.Gosched()
    }
  }
}

func handleSuccessorHeartbeats() {

  for {
    if successorAddr != "" {
      timeout_s := make(chan bool, 1)
      go askIfAlive(timeout_s, successorAddr)

      select {
      case <-successorAliveChannel:
        fmt.Println("Heartbeat from successor: ", successorAddr)
      case <-timeout_s:
        // timed out, my successor might be dead. time to make some changes in our secret circle
        fmt.Println("Timed out on successor heartbeat")
        // locate new successor if any
        // update predecessor of new 
        successor = -1
        successorAddr = ""
        stabilizeNode("successor")
      }     
      time.Sleep(5 * time.Second)
    } else {
        //fmt.Println("Looping until we have both successor and predecessor")
      runtime.Gosched()
    }
  }
}

func stabilizeNode(position string) {
  // ONE SIMPLE WAY: inquire about our failed successors identifier
  // A node who had a predecessor with that identifier is now our new successor
  // But what if two consecutive nodes fail then this wouldnt work in some cases (unless?)

  // ANOTHER WAY (not sure if this'll work but it should): add 1 to our successor's
  // identifier and inquire about this identifier. The node that stores this iden is our
  // new successor but how to find this out? No 'in between' if our successor fails.
  // Using simple identifier subtraction can solve this.

  // send it to the first alive node in the finger table. that node keeps sending it to the predecessor
  // till the predecessor is a dead node (might be the same dead node if only one node died but it may well be a different one)
  // this node with no living predecessor is now our new successor so this node should send a reply back- all well?
  // BUT we need to be able to detect a dead predecessor for this to work (send acks for with every _heartbeat msg?)
  // Think

  for _, addr := range ftab {
    msg := CommandMessage{"_proposal", myAddr, addr, position, strconv.FormatInt(identifier, 10)}
    buf := getJSONBytes(msg)
    sendMessage(addr, buf)
  }

  // also send to predecessor(?)
  msg := CommandMessage{"_proposal", myAddr, predecessorAddr, position, strconv.FormatInt(identifier, 10)}
  buf := getJSONBytes(msg)
  sendMessage(predecessorAddr, buf)

}

func locatePredecessor(conn net.Conn) {
  //fmt.Println("Locating predecessor...")
  msg := CommandMessage{"_locPred", myAddr, "", strconv.FormatInt(identifier, 10), ""}
  msgInJSON, err := json.Marshal(msg)
  checkError(err)
  buf := []byte(msgInJSON)
  _, err = conn.Write(buf)
  checkError(err)
  //fmt.Println("Sent command: ", string(buf[:]))
}

/* Perform recursive search through finger tables to place me at the right spot.
 */
func locateSuccessor(conn net.Conn, id string) {
  // recursive search through finger tables
  // use computeDistBetweenTwoHashes(Key1 string, Key2 string)
  //fmt.Println("Locating successor...")

  // Send a special message of Value "where", so that node knows it wants to find its place in the identifier circle.
  msg := CommandMessage{"_discover", id, "", "", ""}
  msgInJSON, err := json.Marshal(msg)
  checkError(err)
  //fmt.Println("JSON: ", string(msgInJSON))
  buf := []byte(msgInJSON)
  // The listening function should have an interface{} to deal with a "where" message.
  // Handle this write in the listenForControlMessages function
  _, err = conn.Write(buf)
  //fmt.Println("Sent command: ", string(buf[:]))
  checkError(err)

  // The response should hold the conn of the successor.
  // n, err := conn.Read(buf)
  // checkError(err)
  // var successor ConnResponse
  // err = json.Unmarshal(buf[:n], &successor)
  // checkError(err)

  // return successor.Conn
}

func getNodeInfo(nodeAddr string, iden int64) {
  msg := CommandMessage{"_getInfo", nodeAddr, "", "", strconv.FormatInt(iden, 10)}
  jsonMsg, err := json.Marshal(msg)
  checkError(err)
  b := []byte(jsonMsg)
  sendMessage(successorAddr, b)
}

func initFingerTable(conn net.Conn, nodeAddr string) {
  //fmt.Println("Successor Address: ", successorAddr)
  //fmt.Println("Initializing finger table")
  thisIden := getIdentifier(nodeAddr)
  // for every entry in the finger table i.e
  //limit := math.Pow(2, m)
  for i := 0; i < int(m); i++ {
    key := int64( math.Mod( float64(thisIden) + math.Pow(2, float64(i)), math.Pow(2, float64(m)) ) )
    getNodeInfo(nodeAddr, key)
  }
}

func getIdentifier(Key string) int64 {
  id := computeSHA1Hash(Key)
  k := big.NewInt(0)
  if _, ok := k.SetString(id, 16); ok {
    //fmt.Println("Number: ", k)
  } else {
    fmt.Println("Unable to parse into big int")
  }
  //k, err := strconv.ParseInt(id, 16, 64)
  //checkError(err)
  power := int64(math.Pow(2, m))
  ret := (k.Mod(k, big.NewInt(power))).Int64()
  //fmt.Println("Identifier is: ", ret)
  return ret
}

func getVal(Key string) (string, bool) {
  v := ftab[getIdentifier(Key)]
  if v == "" {
    return v, false
  } else {
    return v, true
  }
}


func sendToNextBestNode(KeyIdentifier int64, msg CommandMessage) {
  //KeyIdentifier := getIdentifier(msg.SourceAddr)
  // find node in finger table which is closest to requested Key *BUG*
  var closestNode string
  minDistanceSoFar := int64(math.MaxInt64)
  for nodeIden, nodeAddr := range ftab {
    diff := nodeIden - KeyIdentifier
    // fmt.Println("NodeIden in ftab: ", nodeIden)
    // fmt.Println("KeyIdentifier to find: ", KeyIdentifier)
    // fmt.Println("DIFFERENCE: ", diff)
    // fmt.Println("Min distance so far", minDistanceSoFar)
    if diff < minDistanceSoFar {
      minDistanceSoFar = diff
      closestNode = nodeAddr
    }
  }
  // send message to closestNode
  jsonMsg, err := json.Marshal(msg)
  checkError(err)
  buf := []byte(jsonMsg)
  sendMessage(closestNode, buf)
  // fmt.Println("Dialing to next best node...")
  // conn, _ := net.Dial("udp", closestNode)
  // _, err = conn.Write(buf)
  // checkError(err)
  
}

func sendMessage(addr string, msg []byte) {
  //fmt.Println("Dialing to send message...")
  //fmt.Println("Address to dial: ", addr)
  fmt.Println("Sending Message: ", string(msg))
  conn, err := net.Dial("udp", addr)
  checkError(err)
  defer conn.Close()
  _, err = conn.Write(msg)
  checkError(err)
}

func betweenIdens(suc int64, me int64, iden int64) bool {
  if suc < me {
    //fmt.Println("Successor less than me")
    if iden > me && iden > suc {
      //fmt.Println("Between me and successor!")
      return true
    } else if iden < me && iden < suc {
      //fmt.Println("Between me and successor!")
      return true
    }
  } else if suc > me {
    //fmt.Println("Successor greater than me")
    if iden > me && iden < suc {
      //fmt.Println("Between me and successor!")
      return true
    }
  }
  return false
}

func provideInfo(msg CommandMessage, nodeAddr string) {
  iden, err := strconv.ParseInt(msg.Val, 10, 64)
  checkError(err)
  if betweenIdens(successor, identifier, iden) {
    reply := CommandMessage{"_resInfo", nodeAddr, msg.SourceAddr, msg.Val, successorAddr}
    jsonReply, err := json.Marshal(reply)
    checkError(err)
    b := []byte(jsonReply)
    sendMessage(msg.SourceAddr, b)
  } else if identifier == iden {
    // heloo.. is it me you're looking for
    reply := CommandMessage{"_resInfo", nodeAddr, msg.SourceAddr, msg.Val, nodeAddr}
    jsonReply, err := json.Marshal(reply)
    checkError(err)
    b := []byte(jsonReply)
    sendMessage(msg.SourceAddr, b)
  } else if val, ok := ftab[iden]; ok {
    reply := CommandMessage{"_resInfo", nodeAddr, msg.SourceAddr, msg.Val, val}
    jsonReply, err := json.Marshal(reply)
    checkError(err)
    b := []byte(jsonReply)
    sendMessage(msg.SourceAddr, b)
  } else {
    fmt.Println("Can't provide info, forwarding message to next best node")
    sendToNextBestNode(iden, msg)
  }
}

func sendPredInfo(src string, succ string) {
  responseMsg := CommandMessage{"_resLocPred", myAddr, src, "predecessor", succ}
  resp, err := json.Marshal(responseMsg)
  checkError(err)
  buf := []byte(resp)
  sendMessage(src, buf)
}

/* Only called when there are no nodes yet that are started up. This node becomes the first node.
 */
func startUpSystem(nodeAddr string) {
  // listen and conect to node

  serverAddr, err := net.ResolveUDPAddr("udp", nodeAddr)
  checkError(err)

  go handlePredecessorHeartbeats()
  go handleSuccessorHeartbeats()
  

  fmt.Println("Trying to listen on: ", serverAddr)
  conn, err := net.ListenUDP("udp", serverAddr)
  checkError(err)
  defer conn.Close()

  var msg CommandMessage
  //jsonMsg, err := json.Marshal(msg)
  //checkError(err)
  buf := make([]byte, 2048)

  for {
    fmt.Println("Waiting for packet to arrive on udp port...")
    n, _, err := conn.ReadFromUDP(buf)
    fmt.Println("Received Command: ", string(buf[:n]))
    checkError(err)
    err = json.Unmarshal(buf[:n], &msg)
    //fmt.Println("Cmd: ", msg.Cmd)
    k, err := strconv.ParseInt(msg.Key, 10, 64)
    //checkError(err)

    switch msg.Cmd {
      case "_proposal":
        if msg.Key == "successor" && predecessor == -1 {
          // send a message 
          responseMsg := CommandMessage{"_resProposal", myAddr, msg.SourceAddr, "successor", strconv.FormatInt(identifier, 10)}
          b := getJSONBytes(responseMsg)
          sendMessage(msg.SourceAddr, b)
        } else if predecessor != -1 && predecessorAddr != "" {
          // i have a predecessor, send message to my predecessor, passing along the chain till a node with no predecessor
          b := getJSONBytes(msg)
          sendMessage(predecessorAddr, b)
        }
      case "_resProposal":
        if msg.Key == "successor" && successor == -1 {
          successor, _ = strconv.ParseInt(msg.Val, 10, 64)
          successorAddr = msg.SourceAddr
          fmt.Println("Found new successor with address: ", successorAddr)
          // send a positive msg back so it knows we accepted proposal and it sets its predecessor
          responseMsg := CommandMessage{"_resProposal", myAddr, msg.SourceAddr, "predecessor", strconv.FormatInt(identifier, 10)}
          b := getJSONBytes(responseMsg)
          sendMessage(msg.SourceAddr, b)
        } else if msg.Key == "predecessor" && predecessor == -1 {
          predecessor, _ = strconv.ParseInt(msg.Val, 10, 64)
          predecessorAddr = msg.SourceAddr
          fmt.Println("Found new predecessor with address: ", predecessorAddr)
          // PROBABLY WONT NEED THIS STEP FOR ONE WAY STABILIZATION
          // send a positive msg back so it knows we accepted proposal and it sets its successor if needed
          responseMsg := CommandMessage{"_resProposal", myAddr, msg.SourceAddr, "successor", strconv.FormatInt(identifier, 10)}
          b := getJSONBytes(responseMsg)
          sendMessage(msg.SourceAddr, b)
        } else {
          fmt.Println("Response proposal message discarded")
        }
      case "_heartbeat":
        if msg.SourceAddr == successorAddr {
          // successor is still alive so all good
          fmt.Println("SUCCESSOR ALIVE!")
          successorAliveChannel <- true
        }
        if msg.SourceAddr == predecessorAddr {
          // predecessor is still alive so all good
          fmt.Println("PREDECESSOR ALIVE!")
          predecessorAliveChannel <- true
        }
      case "_alive?":
        // received an alive query - send back message to tell I'm still here
        if predecessorAddr == msg.SourceAddr {
          sendAliveMessage(predecessorAddr)
        } else if successorAddr == msg.SourceAddr {
          sendAliveMessage(successorAddr)
        } else {
            fmt.Println("successorAddr: ", successorAddr)
            fmt.Println("predecessorAddr: ", predecessorAddr)
            fmt.Println("Received alive query from an unexpected node with addr: ", msg.SourceAddr)
        }     
      case "_getInfo":
        provideInfo(msg, nodeAddr)
      case "_getVal":
        v, haveKey := getVal(msg.Key)
        if haveKey {
          // respond with Value
          responseMsg := CommandMessage{"_resVal", nodeAddr, msg.SourceAddr, msg.Key, v}
          resp, err := json.Marshal(responseMsg)
          checkError(err)
          buf = []byte(resp)
          // connect to source of request and send Value
          sendMessage(msg.SourceAddr, buf)
        } else {
          // send to next best node
          sendToNextBestNode(k, msg)
        }
      case "_resInfo":
        ftab[k] = msg.Val
        fmt.Println("Set finger table entry ", msg.Key, " to ", ftab[k])
      case "_setVal":
        _, haveKey := getVal(msg.Key)
        if haveKey {
          // change Value
          store[msg.Key] = msg.Val
          responseMsg := CommandMessage{"_resGen", nodeAddr, msg.SourceAddr, "", "Key Updated"}
          resp, err := json.Marshal(responseMsg)
          checkError(err)
          buf = []byte(resp)
          // connect to source of request and send Value
          sendMessage(msg.SourceAddr, buf)
        } else {
          // send to next best node
          sendToNextBestNode(k,msg)
        }
      case "_locPred" :
        if msg.SourceAddr == successorAddr {
          sendPredInfo(msg.SourceAddr, nodeAddr)
        } else {
          // send to next best node (?)
          sendToNextBestNode(k, msg)
        }
      case "_resLocPred":
        // key in this case would hold asking node's identifier
        predecessor, _ = strconv.ParseInt(msg.Key, 10, 64)
        predecessorAddr = msg.Val;
        fmt.Println("Updated predecessor to: ", predecessorAddr)
      case "_resDisc" :
        successor = getIdentifier(msg.Val)
        successorAddr = msg.Val
        c <- "okay"
        fmt.Println("Successor updated to address: ", msg.Val)
        //fmt.Println("Successor Identifier is: ", successor)
      case "_discover":
        nodeIdentifier := getIdentifier(msg.SourceAddr)
        if successor == -1 {
          fmt.Println("No successor in network. Setting now to new node...")
          ftab[nodeIdentifier] = msg.SourceAddr
          
          // notify new node of its successor (current successor)
          responseMsg := CommandMessage {"_resDisc", nodeAddr, msg.SourceAddr, "", nodeAddr}
          resMsg, err := json.Marshal(responseMsg)
          checkError(err)
          buf := []byte(resMsg)
          sendMessage(msg.SourceAddr, buf)
          // update successor to new node
          successor = nodeIdentifier
          successorAddr = msg.SourceAddr
          // update predecessor too
          predecessorAddr = msg.SourceAddr
          break
        }
        //nodeIdentifier >= identifier && nodeIdentifier <= successor
        //func betweenIdens(suc int64, me int64, iden int64) bool
        if betweenIdens(successor, identifier, nodeIdentifier) {
          // incoming node belongs between this node and its current successor
          // Update current successor's pred to new node
          sendPredInfo(successorAddr, msg.SourceAddr)
          // Update new node's pred to me (do we really need this since new node explicitly asks for pred)
          sendPredInfo(msg.SourceAddr, myAddr)
          fmt.Println("New node fits between me and my successor. Updating finger table...")
          ftab[nodeIdentifier] = msg.SourceAddr
          // notify new node of its successor (current successor)
          responseMsg := CommandMessage {"_resDisc", nodeAddr, msg.SourceAddr, "", successorAddr}
          resMsg, err := json.Marshal(responseMsg)
          checkError(err)
          buf := []byte(resMsg)
          sendMessage(msg.SourceAddr, buf)
          // update successor to new node
          successor = nodeIdentifier
          successorAddr = msg.SourceAddr
          break
        } else {
          // forward command to next best node
          sendToNextBestNode(getIdentifier(msg.SourceAddr), msg)
          break
        }
    }
  }
}

/* Attempt to join the system given the ip:port of a running node.
 */
func connectToSystem(nodeAddr string, startAddr string) {
  // Get this node's IP hash, which will be used as its ID.
  //id := computeSHA1Hash(nodeAddr)
  fmt.Println("Connecting to peer system...")

  //nodeUDPAddr, err := net.ResolveUDPAddr("udp", nodeAddr)
  //checkError(err)
  //startUDPAddr, err := net.ResolveUDPAddr("udp", startAddr)
  //checkError(err)

  // Figure out where I am in the identifier circle.
  conn, err := net.Dial("udp", startAddr)
  checkError(err)

  defer conn.Close()
  //successorConn := locateSuccessor(conn, nodeAddr)
  locateSuccessor(conn, nodeAddr)
  locatePredecessor(conn)
  <-c
  // initialize finger table
  initFingerTable(conn, nodeAddr)
  // Don't need this conn object anymore.
  // if successorConn != conn {
  //   defer conn.Close()
  // }

  // Send a heartbeat every 5 secs. The successor will start tracking this
  // node if it hadn't sent an alive message before.
  //go sendAliveMessage(successorConn, nodeAddr)

  // Listen for TCP message requests to this node.
  //listenForControlMessages(nodeAddr)
}

func getJSONBytes(message CommandMessage) []byte {
  resp, err := json.Marshal(message)
  checkError(err)
  return []byte(resp)
}

/* The main function.
 */
func main() {
  // Handle the command line.
  if len(os.Args) > 5 || len(os.Args) < 3 {
    fmt.Println("Usage: go run node.go [node ip:port] [starter-node ip:port] [-r=replicationFactor] [-t]")
    os.Exit(-1)
  } else {
    myAddr = os.Args[1] // ip:port of this node
    startAddr := os.Args[2] // ip:port of initial node
    flag.IntVar(&replicationFactor, "r", 2, "replication factor")
    flag.BoolVar(&traceMode, "t", false, "trace mode")
    flag.Parse()

    store = make(map[string]string)
    ftab = make(map[int64]string)
    m = 3

    successor = -1
    successorAddr = ""
    predecessor = -1
    predecessorAddr = ""

    c = make(chan string)
    identifier = getIdentifier(myAddr)

    successorAliveChannel = make(chan bool, 1)
    predecessorAliveChannel = make(chan bool, 1)
    //timeout = make(chan bool, 1)

    fmt.Println("THIS NODE'S IDENTIFIER IS: ", identifier)

    // Set both args as equal on the command line if no node is operational yet.
    // Need to start up a source node (this) first.
    // if (nodeAddr == startAddr) {
    //   startUpSystem(nodeAddr)
    // // Else join an existing identifier circle, given the address of a node that is running.
    // } else {
    //   connectToSystem(nodeAddr, startAddr)
    // }
    if (myAddr == startAddr) {
      fmt.Println("First node in system. Listening for incoming connections...")
      go startUpSystem(myAddr)
    } else {
      //go func() {
        go startUpSystem(myAddr)
        go connectToSystem(myAddr, startAddr)
      //}()
    }
  }
  for {
    runtime.Gosched()
  }

  // Setup UDP server
 //  u := transfile.UdpInfo{
 //    Conn: nil,
 //    Port: ":0",
 //  }
 //  u.SetupUdp()
 //  fmt.Println("This UDP:", u)

  
 //  (1) New u.Port is to be advertised as part of finger
 //  (2) Send data like this: u.SendUdpCall("198.162.33.54:40465")
  

	// go u.ReceiveUdpCall() // infinite waiting & receiving
	// transfile.StayLive()
}
