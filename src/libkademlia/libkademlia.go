package libkademlia

// Contains the core kademlia type. In addition to core state, this type serves
// as a receiver for the RPC methods, which is required by that package.

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"net/rpc"
	"strconv"
	"container/list"
	"time"
	"sort"
)

const (
	alpha = 3
	b     = 8 * IDBytes
	k     = 20
)

// Kademlia type. You can put whatever state you need in this.
type Kademlia struct {
	NodeID      	ID
	SelfContact 	Contact
	Table					RoutingTable
	DataStore 		map[ID][]byte
	Channels      KademliaChannels
}

type RoutingTable struct {
	Buckets 			[b]*list.List //160 lists
}

type KademliaChannels struct {
	findContactIncomingChan   chan ID						// testping channels
	findContactOutgoingChan   chan *Contact
	updateContactChannel      chan Contact

	storeReqChannel           chan StoreRequest				// teststore channel

	findValueIncomingChannel  chan ID 						// testfindvalue channels
	findValueOutgoingChannel  chan []byte

	findNodeIncomingChannel   chan ID 						// testfindnode channels
	findNodeOutgoingChannel   chan []Contact

	iterativeFindNodeChan     chan IterativeFindNodeResult	//Project 2
	iterativeFindValueChan 		chan IterativeFindValueResult
}



func NewKademliaWithId(laddr string, nodeID ID) *Kademlia {
	k := new(Kademlia)

	k.Channels = KademliaChannels{
		findContactIncomingChan: 	make(chan ID),
		findContactOutgoingChan: 	make(chan *Contact),
		updateContactChannel:    	make(chan Contact),
		storeReqChannel:					make(chan StoreRequest),
		findValueIncomingChannel:	make(chan ID),
		findValueOutgoingChannel:	make(chan []byte),
		findNodeIncomingChannel:	make(chan ID),
		findNodeOutgoingChannel:	make(chan []Contact),
		iterativeFindNodeChan:		make(chan IterativeFindNodeResult),
		iterativeFindValueChan:		make(chan IterativeFindValueResult),
	}

	k.NodeID = nodeID
	// TODO: Initialize other state here as you add functionality.
	for index, _ := range k.Table.Buckets {
		k.Table.Buckets[index] = list.New()
	}

	go KBucketManager(k)
	go DataStoreManager(k)

	// Set up RPC server
	// NOTE: KademliaRPC is just a wrapper around Kademlia. This type includes
	// the RPC functions.

	s := rpc.NewServer()
	s.Register(&KademliaRPC{k})
	hostname, port, err := net.SplitHostPort(laddr)
	if err != nil {
		return nil
	}
	s.HandleHTTP(rpc.DefaultRPCPath+port,
		rpc.DefaultDebugPath+port)
	l, err := net.Listen("tcp", laddr)
	if err != nil {
		log.Fatal("Listen: ", err)
	}

	// Run RPC server forever.
	go http.Serve(l, nil)

	// Add self contact
	hostname, port, _ = net.SplitHostPort(l.Addr().String())
	port_int, _ := strconv.Atoi(port)
	ipAddrStrings, err := net.LookupHost(hostname)
	var host net.IP
	for i := 0; i < len(ipAddrStrings); i++ {
		host = net.ParseIP(ipAddrStrings[i])
		if host.To4() != nil {
			break
		}
	}
	k.SelfContact = Contact{k.NodeID, host, uint16(port_int)}
	return k
}

func NewKademlia(laddr string) *Kademlia {
	return NewKademliaWithId(laddr, NewRandomID())
}

type ContactNotFoundError struct {
	id  ID
	msg string
}

func (e *ContactNotFoundError) Error() string {
	return fmt.Sprintf("%x %s", e.id, e.msg)
}

func (k *Kademlia) FindContact(nodeId ID) (*Contact, error) {

	// TODO: Search through contacts, find specified ID
	// Find contact with provided ID
	if nodeId == k.SelfContact.NodeID {
		return &k.SelfContact, nil
	}
	k.Channels.findContactIncomingChan <- nodeId

	foundContact := <- k.Channels.findContactOutgoingChan

	if foundContact != nil{
		return foundContact, nil
	}else{
		return nil, &ContactNotFoundError{nodeId, "Not found"}
	}

}


func KBucketManager(kadem *Kademlia) {
	for {
		select {

		/*
		 * Find a contact among our kBuckets
		 */
		case requestedContactID := <-kadem.Channels.findContactIncomingChan:

			//fmt.Println("looking for", requestedContactID.AsString())

			distance := kadem.SelfContact.NodeID.Xor(requestedContactID)
			bucketIdx := distance.PrefixLen()
			bucketIdx = (b - 1) - bucketIdx		// flip it so the largest distance goes in the largest bucket

			//fmt.Println(kadem.SelfContact.NodeID.AsString(), "looking for", requestedContactID.AsString(), "where distance is", distance, "looking in bucket", bucketIdx)

			if bucketIdx >= 0 {
				bucket := kadem.Table.Buckets[bucketIdx]

				//fmt.Println("Bucket length is", bucket.Len())

				contactFound := false
				for e := bucket.Front(); e != nil; e = e.Next() {
					//fmt.Println("Looking...")

					elementID := (e.Value.(*Contact)).NodeID
					//fmt.Println(elementID.AsString())
					if elementID.Equals(requestedContactID){
						contactFound =true
						kadem.Channels.findContactOutgoingChan <- e.Value.(*Contact)
						break
						//TODO: might need to pass this to a go routine later
						//bucket.MoveToBack(e)
					}
				}

				if(!contactFound) {
					//fmt.Println("couldn't find contact")
					kadem.Channels.findContactOutgoingChan <- nil
				}
			}

		case contactToBeUpdated := <-kadem.Channels.updateContactChannel:
			//fmt.Println("KBucketManager is telling", kadem.SelfContact.NodeID.AsString(), "to update", contactToBeUpdated.NodeID.AsString())
			kadem.Update(&contactToBeUpdated)

		case requestedNodesSearchKey := <-kadem.Channels.findNodeIncomingChannel:
			distance := kadem.SelfContact.NodeID.Xor(requestedNodesSearchKey)
			bucketIdx := distance.PrefixLen()
			bucketIdx = (b - 1) - bucketIdx
			//bucket := kadem.Table.Buckets[bucketIdx]
			//fmt.Println("starting index:", bucketIdx)
			contactArray :=make([]Contact,0,k)

			//go up
			for i := bucketIdx; i < b; i++{
				bucket := kadem.Table.Buckets[i]
				for e := bucket.Front(); e != nil; e = e.Next() {
					//fmt.Println(kadem.SelfContact.NodeID.AsString(), "got",bucket.Len(), "contacts in bucket ", i, "contact is: ", (e.Value.(*Contact)).NodeID.AsString())
					if(len(contactArray)>=k){
						break;
					}
					contactArray= append(contactArray, *(e.Value.(*Contact)))
				}
			}

			//go down
			if(len(contactArray)<k){
				for j := bucketIdx; j >=0 ; j--{
					bucket := kadem.Table.Buckets[j]
					for e := bucket.Front(); e != nil; e = e.Next() {
						if(len(contactArray)>=k){
							break;
						}
						contactArray= append(contactArray, *(e.Value.(*Contact)))
					}
				}
			}

			kadem.Channels.findNodeOutgoingChannel <- contactArray
		default:
			//do nothing

		}
	}
}


func DataStoreManager(kadem *Kademlia) {
	kadem.DataStore = make(map[ID][]byte)

	for{
		select{
		case storeReq := <-kadem.Channels.storeReqChannel:
			kadem.DataStore[storeReq.Key] = storeReq.Value
			// fmt.Println("stored stuff", string(kadem.DataStore[storeReq.Key]))
		case key := <-kadem.Channels.findValueIncomingChannel:

			if value, found := kadem.DataStore[key]; found {
				kadem.Channels.findValueOutgoingChannel <- value
			} else {
				kadem.Channels.findValueOutgoingChannel <- nil
			}
		default:
			//do nothing
		}
	}

}

func (kadem *Kademlia) Update(contact *Contact) error {

	// TODO: Implement
	distance := kadem.SelfContact.NodeID.Xor(contact.NodeID)

	// fmt.Println("Distance is")

	bucketIdx := distance.PrefixLen()
	bucketIdx = (b - 1) - bucketIdx		// flip it so the largest distance goes in the largest bucket

	//fmt.Println(kadem.SelfContact.NodeID.AsString(), "inserting", contact.NodeID.AsString(), "into bucket", bucketIdx, "that has length", kadem.Table.Buckets[bucketIdx].Len() )

	if bucketIdx >= 0 {
		bucket := kadem.Table.Buckets[bucketIdx]
		contactExists := false
		for e := bucket.Front(); e != nil; e = e.Next() {
			elementID := (e.Value.(*Contact)).NodeID
			//fmt.Println(elementID.AsString())
			if elementID.Equals(contact.NodeID){
				contactExists = true
				//TODO: might need to pass this to a go routine later
				bucket.MoveToBack(e)
				break
			}
		}

		if !contactExists && bucket.Len() < k{

		//If the contact does not exist and the k-bucket is not full: create a new contact for the node and place at the tail of the k-bucket.
			bucket.PushBack(contact)

			//fmt.Println(kadem.SelfContact.NodeID.AsString(), "has inserted", contact.NodeID.AsString())

			// for e := bucket.Front(); e != nil; e = e.Next() {
			// 	elementID := (e.Value.(*Contact)).NodeID
			// 	fmt.Println(elementID.AsString())
			// }

		} else if !contactExists && bucket.Len() >= k {
			//If the contact does not exist and the k-bucket is full: ping the least recently contacted node (at the head of the k-bucket), if that contact fails to respond, drop it and append new contact to tail, otherwise ignore the new contact and update least recently seen contact.
			frontNode := bucket.Front().Value.(*Contact)
			//fmt.Println(frontNode.NodeID.AsString())
			_, err := kadem.DoPing(frontNode.Host, frontNode.Port)
			if err != nil{
				//we didn't get a response
				bucket.Remove(bucket.Front())
				bucket.PushBack(contact)
			}

		}
	}

	return nil
}

type CommandFailed struct {
	msg string
}

func (e *CommandFailed) Error() string {
	return fmt.Sprintf("%s", e.msg)
}

func (k *Kademlia) DoPing(host net.IP, port uint16) (*Contact, error) {
	// TODO: Implement
	// hostnames,_:=net.LookupAddr(host.String())
	// hostname := hostnames[0]

	hostname:=host.String()
	portString := strconv.Itoa(int(port))
	//log.Println("hostname:",hostname, "port:", portString, "RPCPath:",rpc.DefaultRPCPath+hostname+portString)
	client, err := rpc.DialHTTPPath("tcp", hostname+":"+portString,
		rpc.DefaultRPCPath+portString)
	if err != nil {
		log.Fatal("DialHTTP: ", err)
	}

	ping := new(PingMessage)
	ping.MsgID = NewRandomID()
	ping.Sender = k.SelfContact

	var pong PongMessage

	callRes := client.Go("KademliaRPC.Ping", ping, &pong, nil)

	select {

	case <-callRes.Done:
		// do what you need with test.CallReply
		//log.Printf("ping msgID: %s\n", ping.MsgID.AsString())
		//log.Printf("pong msgID: %s\n\n", pong.MsgID.AsString())
		//fmt.Println("received pong from:", pong.Sender.NodeID.AsString())

		k.Channels.updateContactChannel <- pong.Sender
		return &(pong.Sender), nil

	case <-time.After(5 * time.Second):
		// handle call failing
		return nil, &CommandFailed{"Timeout"}
	}

	//return nil, &CommandFailed{"Unable to ping " + fmt.Sprintf("%s:%v", host.String(), port)}
}

func (k *Kademlia) DoStore(contact *Contact, key ID, value []byte) error {
	// TODO: Implement
	fmt.Printf("Inside Do Store with Contact ID:", contact.NodeID.AsString(), "contact:", contact)

	hostname:=contact.Host.String()
	portString := strconv.Itoa(int(contact.Port))
	//log.Println("hostname:",hostname, "port:", portString, "RPCPath:",rpc.DefaultRPCPath+hostname+portString)
	client, err := rpc.DialHTTPPath("tcp", hostname+":"+portString,
		rpc.DefaultRPCPath+portString)
	if err != nil {
		log.Fatal("DialHTTP: ", err)
	}

	StoreReq := new(StoreRequest)
	StoreReq.MsgID = NewRandomID()
	StoreReq.Sender = k.SelfContact
	StoreReq.Key = key
	StoreReq.Value = value

	var StoreRes StoreResult
	err = client.Call("KademliaRPC.Store", StoreReq, &StoreRes)
	if err != nil {
		log.Fatal("Call: ", err)
	}

	return nil
}

func (k *Kademlia) DoFindNode(contact *Contact, searchKey ID) ([]Contact, error) {
	hostname := contact.Host.String()
	portString := strconv.Itoa(int(contact.Port))

	client, err := rpc.DialHTTPPath("tcp", hostname+":"+portString,
		rpc.DefaultRPCPath+portString)
	if err != nil {
		log.Fatal("DialHTTP: ", err)
	}

	FindNodeReq := new(FindNodeRequest)
	FindNodeReq.MsgID = NewRandomID()
	FindNodeReq.Sender = k.SelfContact
	FindNodeReq.NodeID = searchKey

	var FindNodeRes FindNodeResult
	callRes := client.Go("KademliaRPC.FindNode", FindNodeReq, &FindNodeRes, nil)

	fmt.Println("Inside do find node called by", k.SelfContact.NodeID.AsString(), "on", contact.NodeID.AsString())
	select {

	case <-callRes.Done:
		//fmt.Println("Do Find Node Call Returned")
		for i:=0; i<len(FindNodeRes.Nodes);i++{
			k.Channels.updateContactChannel <- FindNodeRes.Nodes[i]
		}

		return FindNodeRes.Nodes, FindNodeRes.Err

	case <-time.After(300 * time.Millisecond):
		// handle call failing
		return nil, &CommandFailed{"Timeout"}
	}
}

func (k *Kademlia) DoIterativeFindNodeHelper(contact *Contact, searchKey ID) ([]Contact, error) {
	hostname := contact.Host.String()
	portString := strconv.Itoa(int(contact.Port))

	client, err := rpc.DialHTTPPath("tcp", hostname+":"+portString,
		rpc.DefaultRPCPath+portString)
	if err != nil {
		log.Fatal("DialHTTP: ", err)
	}

	FindNodeReq := new(FindNodeRequest)
	FindNodeReq.MsgID = NewRandomID()
	FindNodeReq.Sender = k.SelfContact
	FindNodeReq.NodeID = searchKey

	var FindNodeRes FindNodeResult
	callRes := client.Go("KademliaRPC.FindNode", FindNodeReq, &FindNodeRes, nil)

	fmt.Println("Inside do find node called by", k.SelfContact.NodeID.AsString(), "on", contact.NodeID.AsString())
	select {

	case <-callRes.Done:
		//fmt.Println("Do Find Node Call Returned")
		for i:=0; i<len(FindNodeRes.Nodes);i++{
			k.Channels.updateContactChannel <- FindNodeRes.Nodes[i]
		}

		IterativeFindNodeRes := new(IterativeFindNodeResult)
		IterativeFindNodeRes.MsgID = CopyID(FindNodeRes.MsgID)
		IterativeFindNodeRes.Nodes = FindNodeRes.Nodes
		IterativeFindNodeRes.Err = FindNodeRes.Err
		IterativeFindNodeRes.OriginalRequester = *contact

		k.Channels.iterativeFindNodeChan <- *IterativeFindNodeRes

		return FindNodeRes.Nodes, FindNodeRes.Err

	case <-time.After(300 * time.Millisecond):
		// handle call failing
		IterativeFindNodeResFail := new(IterativeFindNodeResult)
		IterativeFindNodeResFail.MsgID = NewRandomID()
		IterativeFindNodeResFail.Nodes = nil
		IterativeFindNodeResFail.Err = FindNodeRes.Err
		IterativeFindNodeResFail.OriginalRequester = *contact
		k.Channels.iterativeFindNodeChan <- *IterativeFindNodeResFail
		return nil, &CommandFailed{"Timeout"}
	}
}



func (k *Kademlia) DoFindValue(contact *Contact,
	searchKey ID) (value []byte, contacts []Contact, err error) {
	// TODO: Implement
	fmt.Printf("Inside Do Find Value")

	hostname:=contact.Host.String()
	portString := strconv.Itoa(int(contact.Port))
	//log.Println("hostname:",hostname, "port:", portString, "RPCPath:",rpc.DefaultRPCPath+hostname+portString)
	client, err := rpc.DialHTTPPath("tcp", hostname+":"+portString,
		rpc.DefaultRPCPath+portString)
	if err != nil {
		log.Fatal("DialHTTP: ", err)
	}

	FindValReq := new(FindValueRequest)
	FindValReq.MsgID = NewRandomID()
	FindValReq.Sender = k.SelfContact
	FindValReq.Key = searchKey

	var FindValRes FindValueResult
	err = client.Call("KademliaRPC.FindValue", FindValReq, &FindValRes)
	if err != nil {
		log.Fatal("Call: ", err)
	}

	return FindValRes.Value, FindValRes.Nodes, FindValRes.Err
}

func (k *Kademlia) LocalFindValue(searchKey ID) ([]byte, error) {
	// TODO: Implement
	k.Channels.findValueIncomingChannel<-searchKey
	value := <-k.Channels.findValueOutgoingChannel

	return value, nil
}

// For project 2!

//extra fields added
type IterativeFindNodeResult struct {
	MsgID ID
	Nodes []Contact
	Err   error
	OriginalRequester Contact
}

type ShortListContact struct {
	NodeID ID
	Host   net.IP
	Port   uint16
	Distance ID
}

//The interface required so we can sort a list of contacts
type ShortListContacts []ShortListContact

func (slice ShortListContacts) Len() int {
    return len(slice)
}

func (slice ShortListContacts) Less(i, j int) bool {
    return slice[i].Distance.Less(slice[j].Distance);
}

func (slice ShortListContacts) Swap(i, j int) {
    slice[i], slice[j] = slice[j], slice[i]
}

type IterativeFindValueResult struct {
	MsgID ID
	Nodes []Contact
	Value []byte
	Err		error
	OriginalRequester Contact
}

func (kadem *Kademlia) DoIterativeFindNode(id ID) ([]Contact, error) {
	var activeNodes ShortListContacts
	var nodesToVisit ShortListContacts
	visitedNodes := make(map[ID]bool)
	var closestContact Contact

	closestContactChanged := false
	lastIteration := false

	kadem.Channels.findNodeIncomingChannel <- id
	foundNodes := <- kadem.Channels.findNodeOutgoingChannel
	fmt.Println(len(foundNodes))
	//initialize...
	closestContact = foundNodes[0]
	fmt.Println(closestContact.NodeID.AsString())
	//Prepare the first iteration of alpha contacts
	for index, element := range foundNodes {
		if (index >= alpha) { break }

		//create a new short list contact for purposes of sorting
		contactToVisit := new(ShortListContact)
		contactToVisit.NodeID = element.NodeID
		contactToVisit.Host = element.Host
		contactToVisit.Port = element.Port
		contactToVisit.Distance = element.NodeID.Xor(id)

		//add the nodes to the slice of nodes to visit
		nodesToVisit = append(nodesToVisit,*contactToVisit)
		fmt.Println((*contactToVisit).NodeID.AsString(), "appended")
		//update closest contact
		if contactToVisit.Distance.Less(closestContact.NodeID.Xor(id)){
			closestContact = element
		}
	}

	//loop until we have k active contacts, or we no longer improve the list
	for len(activeNodes)<k {

	  	numCalls := 0

		// Iterate over the nodesToVisit and send up to alpha calls, or k calls if in the last iteration
		for _, element := range nodesToVisit {
			if !visitedNodes[element.NodeID]{
				numCalls++
				contactToCall := new(Contact)
				contactToCall.NodeID = element.NodeID
				contactToCall.Host = element.Host
				contactToCall.Port = element.Port
				go kadem.DoIterativeFindNodeHelper(contactToCall, CopyID(id))
				fmt.Println((contactToCall).NodeID.AsString(), "called to find node")
				visitedNodes[element.NodeID] =true
			}

			if !lastIteration {
				if numCalls >= alpha { break }
			}else{
				if numCalls >= k {break}
			}
		}

		//delete nodes from the list?
		nodesToVisit = nodesToVisit[numCalls:]
		fmt.Println("numCalls:", numCalls)

		for ;numCalls > 0;{
			select{
				case foundNodeResult := <- kadem.Channels.iterativeFindNodeChan:
					numCalls--
					fmt.Println("numCalls after decrement:", numCalls)
					fmt.Println(foundNodeResult.OriginalRequester.NodeID.AsString(), "returned from call")
					fmt.Println("Num nodes returned:" ,len(foundNodeResult.Nodes))
					if foundNodeResult.Nodes != nil { // we need to implement such that if timeout, this is nil - node is not active
						activeNode := new(ShortListContact)
						activeNode.NodeID = foundNodeResult.OriginalRequester.NodeID
						activeNode.Host = foundNodeResult.OriginalRequester.Host
						activeNode.Port = foundNodeResult.OriginalRequester.Port
						activeNode.Distance = foundNodeResult.OriginalRequester.NodeID.Xor(id)
						activeNodes = append(activeNodes, *activeNode)

						foundNodes := foundNodeResult.Nodes
						//add foundNodes to list of nodesToVisit
						for _, element := range foundNodes {
							fmt.Println("Node returned with ID:", element.NodeID.AsString())
							if !visitedNodes[element.NodeID]{
								//create a new short list contact for purposes of sorting
								contactToVisit := new(ShortListContact)
								contactToVisit.NodeID = element.NodeID
								contactToVisit.Host = element.Host
								contactToVisit.Port = element.Port
								contactToVisit.Distance = element.NodeID.Xor(id)

								nodesToVisit = append(nodesToVisit, *contactToVisit)

								//update closest contact
								if contactToVisit.Distance.Less(closestContact.NodeID.Xor(id)){
									closestContact = element
									closestContactChanged = true
								}
							}
						}

					} else {
						fmt.Println("Node inactive")
					}

				default:
					//fmt.Println("in default case")
			}
		}

		sort.Sort(nodesToVisit)

		//check to see if this was supposed to be the last iteration
		if lastIteration{
			break
		}

		//check if closest contact changed, if not, begin next cycle
		if closestContactChanged{
			closestContactChanged = false
		}else{
			lastIteration = true //from Piazza, he says we could just stop here after sending the k requests
		}
	}

	//sort active nodes
	sort.Sort(activeNodes)
	//convert back to contacts, return first k
	var shortList []Contact
	for index, element := range activeNodes {
		if (index >= k) { break }
		activeContact := new(Contact)
		activeContact.NodeID = element.NodeID
		activeContact.Host = element.Host
		activeContact.Port = element.Port
		shortList = append(shortList,*activeContact)
		fmt.Println("added node to short list:", activeContact.NodeID.AsString())
	}

	fmt.Println("length of short list:", len(shortList))
	return shortList, nil
}

func (k *Kademlia) DoIterativeStore(key ID, value []byte) ([]Contact, error) {
	contactArray := k.DoIterativeFindNode(key)
	for _, element := range contactArray {
		k.DoStore(element, key , value)
	}

	return contactArray, nil
}

func (kadem *Kademlia) DoIterativeFindValue(key ID) (value []byte, err error) {
	var activeNodes ShortListContacts
	var nodesToVisit ShortListContacts
	visitedNodes := make(map[ID]bool)
	var closestContact Contact

	closestContactChanged := false
	lastIteration := false

	kadem.Channels.findNodeIncomingChannel <- key
	foundNodes := <- kadem.Channels.findNodeOutgoingChannel
	fmt.Println(len(foundNodes))
	//initialize...
	closestContact = foundNodes[0]
	fmt.Println(closestContact.NodeID.AsString())
	//Prepare the first iteration of alpha contacts
	for index, element := range foundNodes {
		if (index >= alpha) { break }

		//create a new short list contact for purposes of sorting
		contactToVisit := new(ShortListContact)
		contactToVisit.NodeID = element.NodeID
		contactToVisit.Host = element.Host
		contactToVisit.Port = element.Port
		contactToVisit.Distance = element.NodeID.Xor(key)

		//add the nodes to the slice of nodes to visit
		nodesToVisit = append(nodesToVisit,*contactToVisit)
		fmt.Println((*contactToVisit).NodeID.AsString(), "appended")
		//update closest contact
		if contactToVisit.Distance.Less(closestContact.NodeID.Xor(key)){
			closestContact = element
		}
	}

	//loop until we have k active contacts, or we no longer improve the list
	for len(activeNodes)<k {

			numCalls := 0

		// Iterate over the nodesToVisit and send up to alpha calls, or k calls if in the last iteration
		for _, element := range nodesToVisit {
			if !visitedNodes[element.NodeID]{
				numCalls++
				contactToCall := new(Contact)
				contactToCall.NodeID = element.NodeID
				contactToCall.Host = element.Host
				contactToCall.Port = element.Port
				go kadem.DoIterativeFindValueHelper(contactToCall, CopyID(key))
				fmt.Println((contactToCall).NodeID.AsString(), "called to find node")
				visitedNodes[element.NodeID] =true
			}

			if !lastIteration {
				if numCalls >= alpha { break }
			}else{
				if numCalls >= k {break}
			}
		}

		//delete nodes from the list?
		nodesToVisit = nodesToVisit[numCalls:]
		fmt.Println("numCalls:", numCalls)

		for ;numCalls > 0;{
			select{
			case foundValueResult := <- kadem.Channels.iterativeFindValueChan:
					numCalls--

					// fmt.Println("numCalls after decrement:", numCalls)
					// fmt.Println(foundNodeResult.OriginalRequester.NodeID.AsString(), "returned from call")
					// fmt.Println("Num nodes returned:" ,len(foundNodeResult.Nodes))

					if foundValueResult.Value != nil {
						return foundValueResult.Value, nil
					}

					if foundValueResult.Nodes != nil { // we need to implement such that if timeout, this is nil - node is not active
						activeNode := new(ShortListContact)
						activeNode.NodeID = foundValueResult.OriginalRequester.NodeID
						activeNode.Host = foundValueResult.OriginalRequester.Host
						activeNode.Port = foundValueResult.OriginalRequester.Port
						activeNode.Distance = foundValueResult.OriginalRequester.NodeID.Xor(key)
						activeNodes = append(activeNodes, *activeNode)

						foundNodes := foundValueResult.Nodes
						//add foundNodes to list of nodesToVisit
						for _, element := range foundNodes {
							fmt.Println("Node returned with ID:", element.NodeID.AsString())
							if !visitedNodes[element.NodeID]{
								//create a new short list contact for purposes of sorting
								contactToVisit := new(ShortListContact)
								contactToVisit.NodeID = element.NodeID
								contactToVisit.Host = element.Host
								contactToVisit.Port = element.Port
								contactToVisit.Distance = element.NodeID.Xor(key)

								nodesToVisit = append(nodesToVisit, *contactToVisit)

								//update closest contact
								if contactToVisit.Distance.Less(closestContact.NodeID.Xor(key)){
									closestContact = element
									closestContactChanged = true
								}
							}
						}

					} else {
						fmt.Println("Node inactive")
					}

				default:
					//fmt.Println("in default case")
			}
		}

		sort.Sort(nodesToVisit)

		//check to see if this was supposed to be the last iteration
		if lastIteration{
			break
		}

		//check if closest contact changed, if not, begin next cycle
		if closestContactChanged{
			closestContactChanged = false
		}else{
			lastIteration = true //from Piazza, he says we could just stop here after sending the k requests
		}
	}

	//sort active nodes
	sort.Sort(activeNodes)
	//convert back to contacts, return first k
	var shortList []Contact
	for index, element := range activeNodes {
		if (index >= k) { break }
		activeContact := new(Contact)
		activeContact.NodeID = element.NodeID
		activeContact.Host = element.Host
		activeContact.Port = element.Port
		shortList = append(shortList,*activeContact)
		fmt.Println("added node to short list:", activeContact.NodeID.AsString())
	}

	fmt.Println("length of short list:", len(shortList))
	return nil, &CommandFailed{"Unable to find value"}
}

func (k *Kademlia) DoIterativeFindValueHelper(contact *Contact,
	searchKey ID) () {

	hostname := contact.Host.String()
	portString := strconv.Itoa(int(contact.Port))

	client, err := rpc.DialHTTPPath("tcp", hostname+":"+portString,
		rpc.DefaultRPCPath+portString)
	if err != nil {
		log.Fatal("DialHTTP: ", err)
	}

	FindValueReq := new(FindValueRequest)
	FindValueReq.MsgID = NewRandomID()
	FindValueReq.Sender = k.SelfContact
	FindValueReq.Key = searchKey

	var FindValueRes FindValueResult
	callRes := client.Go("KademliaRPC.FindValue", FindValueReq, &FindValueRes, nil)

	select {

	case <-callRes.Done:
		IterativeFindValueRes := new(IterativeFindValueResult)
		IterativeFindValueRes.MsgID = CopyID(FindValueRes.MsgID)
		IterativeFindValueRes.Nodes = FindValueRes.Nodes
		IterativeFindValueRes.Value = FindValueRes.Value
		IterativeFindValueRes.Err = FindValueRes.Err
		IterativeFindValueRes.OriginalRequester = *contact

		k.Channels.iterativeFindValueChan <- *IterativeFindValueRes


	case <-time.After(300 * time.Millisecond):
		// handle call failing
		IterativeFindValueResFail := new(IterativeFindValueResult)
		IterativeFindValueResFail.MsgID = NewRandomID()
		IterativeFindValueResFail.Nodes = nil
		IterativeFindValueResFail.Value = nil
		IterativeFindValueResFail.Err = FindValueRes.Err
		IterativeFindValueResFail.OriginalRequester = *contact

		k.Channels.iterativeFindValueChan <- *IterativeFindValueResFail
	}
}

// For project 3!
func (k *Kademlia) Vanish(data []byte, numberKeys byte,
	threshold byte, timeoutSeconds int) (vdo VanashingDataObject) {
	return
}

func (k *Kademlia) Unvanish(searchKey ID) (data []byte) {
	return nil
}
