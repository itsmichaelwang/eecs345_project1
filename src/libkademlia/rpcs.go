package libkademlia

// Contains definitions mirroring the Kademlia spec. You will need to stick
// strictly to these to be compatible with the reference implementation and
// other groups' code.

import (
	"net"
	//"fmt"
)

type KademliaRPC struct {
	kademlia *Kademlia
}

// Host identification.
type Contact struct {
	NodeID ID
	Host   net.IP
	Port   uint16
}

///////////////////////////////////////////////////////////////////////////////
// PING
///////////////////////////////////////////////////////////////////////////////
type PingMessage struct {
	Sender Contact
	MsgID  ID
}

type PongMessage struct {
	MsgID  ID
	Sender Contact
}

/*
 * WHEN I GET A PING
 */
func (k *KademliaRPC) Ping(ping PingMessage, pong *PongMessage) error {
	// TODO: Finish implementation
	//fmt.Println(k.kademlia.SelfContact.NodeID.AsString(), "received a ping from", ping.Sender.NodeID.AsString())


	pong.MsgID = CopyID(ping.MsgID)
	// Specify the sender
	pong.Sender = k.kademlia.SelfContact
	// Update contact, etc
	// TODO: CopyID or reference directly?


	//fmt.Println("I am", k.kademlia.SelfContact.NodeID.AsString(), "and I am about to send to updateContactChannel", ping.Sender.NodeID.AsString())
	k.kademlia.Channels.updateContactChannel<-ping.Sender


	return nil

}

///////////////////////////////////////////////////////////////////////////////
// STORE
///////////////////////////////////////////////////////////////////////////////
type StoreRequest struct {
	Sender Contact
	MsgID  ID
	Key    ID
	Value  []byte
}

type StoreResult struct {
	MsgID ID
	Err   error
}

func (k *KademliaRPC) Store(req StoreRequest, res *StoreResult) error {
	// TODO: Implement.
	//fmt.Println("RPC Store got called from Sender", req.Sender.NodeID.AsString())
	k.kademlia.Channels.storeReqChannel <- req
	return nil
}

///////////////////////////////////////////////////////////////////////////////
// FIND_NODE
///////////////////////////////////////////////////////////////////////////////
type FindNodeRequest struct {
	Sender Contact
	MsgID  ID
	NodeID ID
}

type FindNodeResult struct {
	MsgID ID
	Nodes []Contact
	Err   error
}

func (k *KademliaRPC) FindNode(req FindNodeRequest, res *FindNodeResult) error {
	// TODO: Implement.
	k.kademlia.Channels.findNodeIncomingChannel <- req.NodeID
	foundNodes := <-k.kademlia.Channels.findNodeOutgoingChannel
	if foundNodes != nil{
		res.MsgID = CopyID(req.MsgID)
		res.Nodes = foundNodes
		res.Err = nil
	}else{
		res.MsgID = CopyID(req.MsgID)
		res.Nodes = nil
		res.Err = &CommandFailed{"Could not find nodes"}
	}

	return nil
}

///////////////////////////////////////////////////////////////////////////////
// FIND_VALUE
///////////////////////////////////////////////////////////////////////////////
type FindValueRequest struct {
	Sender Contact
	MsgID  ID
	Key    ID
}

// If Value is nil, it should be ignored, and Nodes means the same as in a
// FindNodeResult.
type FindValueResult struct {
	MsgID ID
	Value []byte
	Nodes []Contact
	Err   error
}

func (k *KademliaRPC) FindValue(req FindValueRequest, res *FindValueResult) error {
	// TODO: Implement.
	//fmt.Println("RPC FindValue got called from Sender", req.Sender.NodeID.AsString())
	k.kademlia.Channels.findValueIncomingChannel <- req.Key

	foundValue := <-k.kademlia.Channels.findValueOutgoingChannel
	if foundValue != nil{
		res.MsgID = CopyID(req.MsgID)
		res.Value = foundValue
		res.Nodes = nil
		res.Err = nil
	}else{
		k.kademlia.Channels.findNodeIncomingChannel <- req.Key
		foundNodes := <-k.kademlia.Channels.findNodeOutgoingChannel
		if foundNodes != nil{
			res.MsgID = CopyID(req.MsgID)
			res.Value = nil
			res.Nodes = foundNodes
			res.Err = nil
		}else{
			res.MsgID = CopyID(req.MsgID)
			res.Value = nil
			res.Nodes = nil
			res.Err = &CommandFailed{"Could not find nodes"}
		}
	}


	return nil
}

// For Project 3

type GetVDORequest struct {
	Sender Contact
	VdoID  ID
	MsgID  ID
}

type GetVDOResult struct {
	MsgID ID
	VDO   VanashingDataObject
}

func (k *KademliaRPC) GetVDO(req GetVDORequest, res *GetVDOResult) error {
	// TODO: Implement.
	return nil
}
