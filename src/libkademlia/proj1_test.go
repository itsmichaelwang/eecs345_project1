package libkademlia

import (
	"bytes"
	"net"
	"strconv"
	"testing"
	"fmt"
	//"time"
)

func StringToIpPort(laddr string) (ip net.IP, port uint16, err error) {
	hostString, portString, err := net.SplitHostPort(laddr)
	if err != nil {
		return
	}
	ipStr, err := net.LookupHost(hostString)
	if err != nil {
		return
	}
	for i := 0; i < len(ipStr); i++ {
		ip = net.ParseIP(ipStr[i])
		if ip.To4() != nil {
			break
		}
	}
	portInt, err := strconv.Atoi(portString)
	port = uint16(portInt)
	return
}

func TestPing(t *testing.T) {
	instance1 := NewKademlia("localhost:7890")
	instance2 := NewKademlia("localhost:7891")

	fmt.Println("TestPing instance1 ID:" , instance1.SelfContact.NodeID.AsString())
	fmt.Println("TestPing instance2 ID:" , instance2.SelfContact.NodeID.AsString())
	fmt.Println()

	// fmt.Println("Running test #1")
	// fmt.Println()

	host2, port2, _ := StringToIpPort("localhost:7891")
	contact2, err := instance2.FindContact(instance2.NodeID)
	if err != nil {
		t.Error("A node cannot find itself's contact info")
	}

	// fmt.Println("Running test #2")
	// fmt.Println()

	contact2, err = instance2.FindContact(instance1.NodeID)
	if err == nil {
		t.Error("Instance 2 should not be able to find instance " +
			"1 in its buckets before ping instance 1")
	}

	fmt.Println("Running test #3")
	fmt.Println()

	fmt.Println(instance1.SelfContact.NodeID.AsString(), "DoPing", instance2.SelfContact.NodeID.AsString())
	instance1.DoPing(host2, port2)

	fmt.Println(instance1.SelfContact.NodeID.AsString(), "FindContact", instance2.SelfContact.NodeID.AsString())
	contact2, err = instance1.FindContact(instance2.NodeID)
	if err != nil {
		t.Error("Instance 2's contact not found in Instance 1's contact list")
		return
	}

	fmt.Println("Running test #4")
	fmt.Println()

	wrong_ID := NewRandomID()
	_, err = instance2.FindContact(wrong_ID)
	if err == nil {
		t.Error("Instance 2 should not be able to find a node with the wrong ID")
	}

	fmt.Println("Running test #5")
	fmt.Println()

	contact1, err := instance2.FindContact(instance1.NodeID)
	if err != nil {
		t.Error("Instance 1's contact not found in Instance 2's contact list")
		return
	}

	if contact1.NodeID != instance1.NodeID {
		t.Error("Instance 1 ID incorrectly stored in Instance 2's contact list")
	}

	if contact2.NodeID != instance2.NodeID {
		t.Error("Instance 2 ID incorrectly stored in Instance 1's contact list")
	}

	return
}

func TestStore(t *testing.T) {
	// test Dostore() function and LocalFindValue() function
	instance1 := NewKademlia("localhost:7892")
	instance2 := NewKademlia("localhost:7893")
	host2, port2, _ := StringToIpPort("localhost:7893")
	instance1.DoPing(host2, port2)
	contact2, err := instance1.FindContact(instance2.NodeID)
	if err != nil {
		t.Error("Instance 2's contact not found in Instance 1's contact list")
		return
	}
	key := NewRandomID()
	value := []byte("Hello World")
	err = instance1.DoStore(contact2, key, value)
	if err != nil {
		t.Error("Can not store this value")
	}
	storedValue, err := instance2.LocalFindValue(key)
	if err != nil {
		t.Error("Stored value not found!")
	}
	if !bytes.Equal(storedValue, value) {
		t.Error("Stored value did not match found value")
	}
	return
}

func TestFindNode(t *testing.T) {
	// tree structure;
	// A->B->tree
	/*
	         C
	      /
	  A-B -- D
	      \
	         E
	*/
	instance1 := NewKademlia("localhost:7894")
	instance2 := NewKademlia("localhost:7895")
	host2, port2, _ := StringToIpPort("localhost:7895")
	instance1.DoPing(host2, port2)
	contact2, err := instance1.FindContact(instance2.NodeID)
	if err != nil {
		t.Error("Instance 2's contact not found in Instance 1's contact list")
		return
	}
	tree_node := make([]*Kademlia, 10)
	for i := 0; i < 10; i++ {
		address := "localhost:" + strconv.Itoa(7896+i)
		tree_node[i] = NewKademlia(address)
		host_number, port_number, _ := StringToIpPort(address)
		instance2.DoPing(host_number, port_number)
	}
	key := NewRandomID()
	contacts, err := instance1.DoFindNode(contact2, key)
	if err != nil {
		t.Error("Error doing FindNode")
	}

	if contacts == nil || len(contacts) == 0 {
		t.Error("No contacts were found")
	}
	// TODO: Check that the correct contacts were stored
	//       (and no other contacts)

	return
}

func TestFindValue(t *testing.T) {
	// tree structure;
	// A->B->tree
	/*
	         C
	      /
	  A-B -- D
	      \
	         E
	*/
	instance1 := NewKademlia("localhost:7926")
	instance2 := NewKademlia("localhost:7927")
	fmt.Println("TestFindValue instance1 ID:" , instance1.SelfContact.NodeID.AsString())
	fmt.Println("TestFindValue instance2 ID:" , instance2.SelfContact.NodeID.AsString())
	fmt.Println()
	host2, port2, _ := StringToIpPort("localhost:7927")
	instance1.DoPing(host2, port2)
	contact2, err := instance1.FindContact(instance2.NodeID)
	if err != nil {
		t.Error("Instance 2's contact not found in Instance 1's contact list")
		return
	}

	tree_node := make([]*Kademlia, 10)
	for i := 0; i < 10; i++ {
		address := "localhost:" + strconv.Itoa(7928+i)
		tree_node[i] = NewKademlia(address)
		host_number, port_number, _ := StringToIpPort(address)
		instance2.DoPing(host_number, port_number)
	}

	key := NewRandomID()
	value := []byte("Hello world")
	err = instance2.DoStore(contact2, key, value)
	if err != nil {
		t.Error("Could not store value")
	}

	// Given the right keyID, it should return the value
	foundValue, contacts, err := instance1.DoFindValue(contact2, key)
	if !bytes.Equal(foundValue, value) {
		t.Error("Stored value did not match found value")
	}

	//Given the wrong keyID, it should return k nodes.
	wrongKey := NewRandomID()
	foundValue, contacts, err = instance1.DoFindValue(contact2, wrongKey)
	if contacts == nil || len(contacts) < 10 {
		t.Error("Searching for a wrong ID did not return contacts" + strconv.Itoa(len(contacts)))
	}

	// TODO: Check that the correct contacts were stored
	//       (and no other contacts)
}


//Explanations:
/*
	- We test the iterative versions in a similar way to the tests for project 1
	- for do iterative find node, we first create the nodes, then do an iterative find to see if we're returned an array of nodes
	- for do iterative find value, we first store a value in one of the nodes, then do an iterative find to see if we can retrieve that value
	- for do iterative store, we store a value in a node and later check using a local find value to see if it was successfully stored.
*/


//Project 2 Tests


func TestIterativeFindNode(t *testing.T) {
	// tree structure;
	// A->B->tree
	/*
	         C
	      /
	  A-B -- D
	      \
	         E
	*/
	instance1 := NewKademlia("localhost:8000")
	instance2 := NewKademlia("localhost:8001")
	fmt.Println("Instance 1: ", instance1.NodeID.AsString())
	fmt.Println("Instance 2: ", instance2.NodeID.AsString())
	host2, port2, _ := StringToIpPort("localhost:8001")
	instance1.DoPing(host2, port2)
	_, err := instance1.FindContact(instance2.NodeID)
	if err != nil {
		t.Error("Instance 2's contact not found in Instance 1's contact list")
		return
	}
	tree_node := make([]*Kademlia, 20)
	for i := 0; i < 20; i++ {
		address := "localhost:" + strconv.Itoa(8002+i)
		tree_node[i] = NewKademlia(address)
		host_number, port_number, _ := StringToIpPort(address)
		fmt.Println("New Node added to instance 2:", tree_node[i].NodeID.AsString())
		instance2.DoPing(host_number, port_number)
	}
	key := NewRandomID()
	contacts, err := instance1.DoIterativeFindNode(key)
	if err != nil {
		t.Error("Error doing FindNode")
	}

	if contacts == nil || len(contacts) == 0 {
		t.Error("No contacts were found")
	}
	// TODO: Check that the correct contacts were stored
	//       (and no other contacts)

	return
}

func TestIterativeStore(t *testing.T) {
	// test Dostore() function and LocalFindValue() function
	instance1 := NewKademlia("localhost:8200")
	instance2 := NewKademlia("localhost:8201")
	host2, port2, _ := StringToIpPort("localhost:8201")
	instance1.DoPing(host2, port2)
	_, err := instance1.FindContact(instance2.NodeID)
	if err != nil {
		t.Error("Instance 2's contact not found in Instance 1's contact list")
		return
	}
	key := NewRandomID()
	value := []byte("Hello World")
	_, err = instance1.DoIterativeStore(key, value)
	if err != nil {
		t.Error("Can not store this value")
	}
	storedValue, err := instance2.LocalFindValue(key)
	if err != nil {
		t.Error("Stored value not found!")
	}
	if !bytes.Equal(storedValue, value) {
		t.Error("Stored value did not match found value")
	}
	return
}


func TestIterativeFindValue(t *testing.T) {
	// tree structure;
	// A->B->tree
	/*
	         C
	      /
	  A-B -- D
	      \
	         E
	*/
	instance1 := NewKademlia("localhost:8500")
	instance2 := NewKademlia("localhost:8501")
	fmt.Println("TestFindValue instance1 ID:" , instance1.SelfContact.NodeID.AsString())
	fmt.Println("TestFindValue instance2 ID:" , instance2.SelfContact.NodeID.AsString())
	fmt.Println()
	host2, port2, _ := StringToIpPort("localhost:8501")
	instance1.DoPing(host2, port2)
	contact2, err := instance1.FindContact(instance2.NodeID)
	if err != nil {
		t.Error("Instance 2's contact not found in Instance 1's contact list")
		return
	}

	tree_node := make([]*Kademlia, 10)
	for i := 0; i < 10; i++ {
		address := "localhost:" + strconv.Itoa(8502+i)
		tree_node[i] = NewKademlia(address)
		host_number, port_number, _ := StringToIpPort(address)
		instance2.DoPing(host_number, port_number)
	}

	key := NewRandomID()
	value := []byte("Hello world")
	err = instance2.DoStore(contact2, key, value)
	if err != nil {
		t.Error("Could not store value")
	}

	// Given the right keyID, it should return the value
	foundValue, err := instance1.DoIterativeFindValue(key)
	if !bytes.Equal(foundValue, value) {
		t.Error("Stored value did not match found value")
	}

	//Given the wrong keyID, it should return k nodes.
	wrongKey := NewRandomID()
	foundValue, err = instance1.DoIterativeFindValue(wrongKey)
	if foundValue != nil {
		t.Error("Searching for a wrong ID somehow found things")
	}

	// TODO: Check that the correct contacts were stored
	//       (and no other contacts)
}


//project 3 test

func TestVanish(t *testing.T) {
	// tree structure;
	// A->B->tree
	/*
	         C
	      /
	  A-B -- D
	      \
	         E
	*/
	instance1 := NewKademlia("localhost:9000")
	instance2 := NewKademlia("localhost:9001")
	fmt.Println("TestFindValue instance1 ID:" , instance1.SelfContact.NodeID.AsString())
	fmt.Println("TestFindValue instance2 ID:" , instance2.SelfContact.NodeID.AsString())

	host2, port2, _ := StringToIpPort("localhost:9001")
	instance1.DoPing(host2, port2)
	_, err := instance1.FindContact(instance2.NodeID)
	if err != nil {
		t.Error("Instance 2's contact not found in Instance 1's contact list")
		return
	}

	tree_node := make([]*Kademlia, 5)
	for i := 0; i < 5; i++ {
		address := "localhost:" + strconv.Itoa(9002+i)
		tree_node[i] = NewKademlia(address)
		//fmt.Println("added node:", tree_node[i].SelfContact.NodeID.AsString())
		host_number, port_number, _ := StringToIpPort(address)
		instance2.DoPing(host_number, port_number)
	}

	//vanish it
	vdoID := NewRandomID()
	data := []byte("Hello data")
	numberKeys := byte(5)
	threshold := byte(2)
	timeoutSeconds :=0
	instance2.Vanish(vdoID,data,numberKeys,threshold,timeoutSeconds)

	//unvanish it
	//ask instance 1 to unvanish it
	
	returnedData := instance1.Unvanish(instance2.NodeID, vdoID)

	// It should be the same
	if !bytes.Equal(data, returnedData) {
		t.Error("Stored value did not match found value")
		fmt.Println(string(data), string(returnedData))
	}

	fmt.Println("outside", string(data), string(returnedData))
	


}

