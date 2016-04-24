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
	Table			RoutingTable
	DataStore 		map[ID][]byte
	Channels        KademliaChannels
}

type RoutingTable struct {
	Buckets 			[b]*list.List //160 lists
}

type KademliaChannels struct {
	findContactIncomingChan 	chan ID							// testping channels
	findContactOutgoingChan   chan *Contact
	updateContactChannel      chan Contact

	storeReqChannel						chan StoreRequest		// teststore channel

	findValueIncomingChannel	chan ID 						//testfindvalue channels
	findValueOutgoingChannel	chan []byte

	findNodeIncomingChannel		chan ID 						//testfindnode channels
	findNodeOutgoingChannel     chan []Contact
}

// var findContactIncomingChan = make(chan ID)
// var findContactOutgoingChan = make(chan *Contact)
// var updateContactChannel = make(chan Contact)

// var storeReqChannel = make(chan StoreRequest)
// var findValueIncomingChannel = make(chan FindValueRequest)
// var findValueOutgoingChannel = make(chan []byte)

//var findNodeIncomingChannel = make(chan ID)
//var findNodeOutgoingChannel = make(chan []Contact)

func NewKademliaWithId(laddr string, nodeID ID) *Kademlia {
	k := new(Kademlia)

	k.Channels = KademliaChannels{
		findContactIncomingChan: 	make(chan ID),
		findContactOutgoingChan: 	make(chan *Contact),
		updateContactChannel:    	make(chan Contact),

		storeReqChannel:			make(chan StoreRequest),
		findValueIncomingChannel:	make(chan ID),
		findValueOutgoingChannel:	make(chan []byte),

		findNodeIncomingChannel:	make(chan ID),
		findNodeOutgoingChannel:	make(chan []Contact),
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

			fmt.Println("looking for", requestedContactID.AsString())

			distance := kadem.SelfContact.NodeID.Xor(requestedContactID)
			bucketIdx := distance.PrefixLen()
			bucketIdx = (b - 1) - bucketIdx		// flip it so the largest distance goes in the largest bucket

			fmt.Println(kadem.SelfContact.NodeID.AsString(), "looking for", requestedContactID.AsString(), "where distance is", distance, "looking in bucket", bucketIdx)

			if bucketIdx >= 0 {
				bucket := kadem.Table.Buckets[bucketIdx]

				fmt.Println("Bucket length is", bucket.Len())

				contactFound := false
				for e := bucket.Front(); e != nil; e = e.Next() {
					fmt.Println("Looking...")

					elementID := (e.Value.(*Contact)).NodeID
					fmt.Println(elementID.AsString())
					if elementID.Equals(requestedContactID){
						contactFound =true
						kadem.Channels.findContactOutgoingChan <- e.Value.(*Contact)
						break
						//TODO: might need to pass this to a go routine later
						//bucket.MoveToBack(e)
					}
				}

				if(!contactFound) {
					fmt.Println("couldn't find contact")
					kadem.Channels.findContactOutgoingChan <- nil
				}
			}

		case contactToBeUpdated := <-kadem.Channels.updateContactChannel:
			fmt.Println("KBucketManager is telling", kadem.SelfContact.NodeID.AsString(), "to update", contactToBeUpdated.NodeID.AsString())
			kadem.Update(&contactToBeUpdated)

		case requestedNodesSearchKey := <-kadem.Channels.findNodeIncomingChannel:
			distance := kadem.SelfContact.NodeID.Xor(requestedNodesSearchKey)
			bucketIdx := distance.PrefixLen()
			bucketIdx = (b - 1) - bucketIdx
			//bucket := kadem.Table.Buckets[bucketIdx]
			fmt.Println("starting index:", bucketIdx)
			contactArray :=make([]Contact,0,k)

			//go up
			for i := bucketIdx; i < b; i++{
				bucket := kadem.Table.Buckets[i]
				for e := bucket.Front(); e != nil; e = e.Next() {
					fmt.Println(kadem.SelfContact.NodeID.AsString(), "got",bucket.Len(), "contacts in bucket ", i, "contact is: ", (e.Value.(*Contact)).NodeID.AsString())
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

	fmt.Println(kadem.SelfContact.NodeID.AsString(), "inserting", contact.NodeID.AsString(), "into bucket", bucketIdx, "that has length", kadem.Table.Buckets[bucketIdx].Len() )

	if bucketIdx >= 0 {
		bucket := kadem.Table.Buckets[bucketIdx]
		contactExists := false
		for e := bucket.Front(); e != nil; e = e.Next() {
			elementID := (e.Value.(*Contact)).NodeID
			fmt.Println(elementID.AsString())
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

			fmt.Println(kadem.SelfContact.NodeID.AsString(), "has inserted", contact.NodeID.AsString())

			// for e := bucket.Front(); e != nil; e = e.Next() {
			// 	elementID := (e.Value.(*Contact)).NodeID
			// 	fmt.Println(elementID.AsString())
			// }

		} else if !contactExists && bucket.Len() >= k {
			//If the contact does not exist and the k-bucket is full: ping the least recently contacted node (at the head of the k-bucket), if that contact fails to respond, drop it and append new contact to tail, otherwise ignore the new contact and update least recently seen contact.
			frontNode := bucket.Front().Value.(*Contact)
			fmt.Println(frontNode.NodeID.AsString())
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
	log.Println("hostname:",hostname, "port:", portString, "RPCPath:",rpc.DefaultRPCPath+hostname+portString)
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
		log.Printf("ping msgID: %s\n", ping.MsgID.AsString())
		log.Printf("pong msgID: %s\n\n", pong.MsgID.AsString())
		fmt.Println("received pong from:", pong.Sender.NodeID.AsString())

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
	// TODO: Implement
	fmt.Printf("Inside Do Find Node")

	hostname:=contact.Host.String()
	portString := strconv.Itoa(int(contact.Port))
	//log.Println("hostname:",hostname, "port:", portString, "RPCPath:",rpc.DefaultRPCPath+hostname+portString)
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
	err = client.Call("KademliaRPC.FindNode", FindNodeReq, &FindNodeRes)
	if err != nil {
		log.Fatal("Call: ", err)
	}

	for i:=0; i<len(FindNodeRes.Nodes);i++{
		k.Channels.updateContactChannel <- FindNodeRes.Nodes[i]
	}

	return FindNodeRes.Nodes, FindNodeRes.Err

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
func (k *Kademlia) DoIterativeFindNode(id ID) ([]Contact, error) {
	return nil, &CommandFailed{"Not implemented"}
}
func (k *Kademlia) DoIterativeStore(key ID, value []byte) ([]Contact, error) {
	return nil, &CommandFailed{"Not implemented"}
}
func (k *Kademlia) DoIterativeFindValue(key ID) (value []byte, err error) {
	return nil, &CommandFailed{"Not implemented"}
}

// For project 3!
func (k *Kademlia) Vanish(data []byte, numberKeys byte,
	threshold byte, timeoutSeconds int) (vdo VanashingDataObject) {
	return
}

func (k *Kademlia) Unvanish(searchKey ID) (data []byte) {
	return nil
}
