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
	Table					RoutingTable
}

type RoutingTable struct {
	Buckets 			[b]*list.List //160 lists
}


func NewKademliaWithId(laddr string, nodeID ID) *Kademlia {
	k := new(Kademlia)
	k.NodeID = nodeID

	// TODO: Initialize other state here as you add functionality.
	for index, _ := range k.Table.Buckets {
		k.Table.Buckets[index] = list.New()
	}

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
	return nil, &ContactNotFoundError{nodeId, "Not found"}
}

func (kadem *Kademlia) Update(contact *Contact) error {
	// TODO: Implement
	distance := kadem.SelfContact.NodeID.Xor(contact.NodeID)
	bucketIdx := distance.PrefixLen()
	bucketIdx = (b - 1) - bucketIdx		// flip it so the largest distance goes in the largest bucket

	fmt.Println("Self ID: ", kadem.SelfContact.NodeID.AsString())
	fmt.Println("Updated ID: ", contact.NodeID.AsString())
	fmt.Println("In bucket:", bucketIdx)

	if bucketIdx >= 0 && !kadem.SelfContact.NodeID.Equals(contact.NodeID){
		bucket := kadem.Table.Buckets[bucketIdx]
		contactExists := false

		for e := bucket.Front(); e != nil; e = e.Next() {
			elementID := (e.Value.(*Contact)).NodeID
			fmt.Println(elementID.AsString())
			//If the contact exists, move the contact to the end of the k-bucket.
			if elementID.Equals(contact.NodeID){
				contactExists = true
				bucket.MoveToBack(e)
				break
			}
		}

		if !contactExists && bucket.Len() < k{
			//If the contact does not exist and the k-bucket is not full: create a new contact for the node and place at the tail of the k-bucket.
			bucket.PushBack(contact)
		} else if !contactExists && bucket.Len() >= k {
			//If the contact does not exist and the k-bucket is full: ping the least recently contacted node (at the head of the k-bucket), if that contact fails to respond, drop it and append new contact to tail, otherwise ignore the new contact and update least recently seen contact.
			frontNode := bucket.Front().Value.(*Contact)
			fmt.Println(frontNode.NodeID.AsString())

			//timeout code
			timeout := make(chan bool, 1)
			go func() {
			    time.Sleep(1 * time.Second)
			    timeout <- true
			}()
			select {
			case <-timeout:
			    // the read from ch has timed out
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
	//hostnames,_:=net.LookupAddr(host.String())
	//hostname := hostnames[0]
	hostname:=host.String()
	portString := strconv.Itoa(int(port))
	log.Println("hostname:",hostname, "port:", portString, "RPCPath:",rpc.DefaultRPCPath+hostname+portString)
	client, err := rpc.DialHTTPPath("tcp", hostname+":"+portString,
		rpc.DefaultRPCPath+portString)
	if err != nil {
		log.Fatal("DialHTTP: ", err)
	}

	log.Printf("Pinging initial peer from DoPing\n")

	ping := new(PingMessage)
	ping.MsgID = NewRandomID()
	ping.Sender = k.SelfContact
	
	var pong PongMessage
	err = client.Call("KademliaRPC.Ping", ping, &pong)
	if err != nil {
		log.Fatal("Call: ", err)
	}
	log.Printf("ping msgID: %s\n", ping.MsgID.AsString())
	log.Printf("pong msgID: %s\n\n", pong.MsgID.AsString())

	return nil, &CommandFailed{
		"Unable to ping " + fmt.Sprintf("%s:%v", host.String(), port)}
}

func (k *Kademlia) DoStore(contact *Contact, key ID, value []byte) error {
	// TODO: Implement
	return &CommandFailed{"Not implemented"}
}

func (k *Kademlia) DoFindNode(contact *Contact, searchKey ID) ([]Contact, error) {
	// TODO: Implement
	return nil, &CommandFailed{"Not implemented"}
}

func (k *Kademlia) DoFindValue(contact *Contact,
	searchKey ID) (value []byte, contacts []Contact, err error) {
	// TODO: Implement
	return nil, nil, &CommandFailed{"Not implemented"}
}

func (k *Kademlia) LocalFindValue(searchKey ID) ([]byte, error) {
	// TODO: Implement
	return []byte(""), &CommandFailed{"Not implemented"}
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
