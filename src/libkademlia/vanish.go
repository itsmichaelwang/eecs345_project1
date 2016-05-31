package libkademlia

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"io"
	mathrand "math/rand"
	"time"
	"sss"
	"fmt"
)

type VanashingDataObject struct {
	AccessKey  int64
	Ciphertext []byte
	NumberKeys byte
	Threshold  byte
}

type VDOStoreReq struct {
	Vdo VanashingDataObject
	Key ID
}

func GenerateRandomCryptoKey() (ret []byte) {
	for i := 0; i < 32; i++ {
		ret = append(ret, uint8(mathrand.Intn(256)))
	}
	return
}

func GenerateRandomAccessKey() (accessKey int64) {
	r := mathrand.New(mathrand.NewSource(time.Now().UnixNano()))
	accessKey = r.Int63()
	return
}

func CalculateSharedKeyLocations(accessKey int64, count int64) (ids []ID) {
	r := mathrand.New(mathrand.NewSource(accessKey))
	ids = make([]ID, count)
	for i := int64(0); i < count; i++ {
		for j := 0; j < IDBytes; j++ {
			ids[i][j] = uint8(r.Intn(256))
		}
	}
	return
}

func encrypt(key []byte, text []byte) (ciphertext []byte) {
	block, err := aes.NewCipher(key)
	if err != nil {
		panic(err)
	}
	ciphertext = make([]byte, aes.BlockSize+len(text))
	iv := ciphertext[:aes.BlockSize]
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		panic(err)
	}
	stream := cipher.NewCFBEncrypter(block, iv)
	stream.XORKeyStream(ciphertext[aes.BlockSize:], text)
	return
}

func decrypt(key []byte, ciphertext []byte) (text []byte) {
	block, err := aes.NewCipher(key)
	if err != nil {
		panic(err)
	}
	if len(ciphertext) < aes.BlockSize {
		panic("ciphertext is not long enough")
	}
	iv := ciphertext[:aes.BlockSize]
	ciphertext = ciphertext[aes.BlockSize:]

	stream := cipher.NewCFBDecrypter(block, iv)
	stream.XORKeyStream(ciphertext, ciphertext)
	return ciphertext
}

func (kadem *Kademlia) VanishData(vdoID ID, data []byte, numberKeys byte,
	threshold byte, timeoutSeconds int) (vdo VanashingDataObject) {
	//generate crypto key
	cryptoKey := GenerateRandomCryptoKey()
	fmt.Println("cryptokey:",cryptoKey)
	//encrypt data
	ciphertext:= encrypt(cryptoKey, data)

	// Split the given secret into N shares of which K are required to recover the secret. Returns a map of share IDs (1-255) to shares.
	keyIdMap, err := sss.Split(numberKeys, threshold, cryptoKey)

	// create an access key (L)
	accessKey := GenerateRandomAccessKey()

	//calculate where to store the split keys
	locationsToStoreKeyIn := CalculateSharedKeyLocations(accessKey, int64(numberKeys)) // look at second argument - correct according to Piazza

	//store shared keys in kademlia network
	idx := 0

	for key, value := range keyIdMap {
		fmt.Println("inside loop")
		all := append([]byte{key}, value...)
		closestContacts, err := kadem.DoIterativeFindNode(locationsToStoreKeyIn[idx])
		if err == nil{
			kadem.DoStore(&(closestContacts[0]),locationsToStoreKeyIn[idx],all) //store it in the closest contact
			fmt.Println ("stored", all, "in", closestContacts[0].NodeID.AsString())
		}
		idx++
	}

	fmt.Println("inside vanishdata",err)

	//create new VDO Object
	vdo = *new(VanashingDataObject)
	vdo.AccessKey = accessKey
	vdo.Ciphertext = ciphertext
	vdo.NumberKeys = numberKeys
	vdo.Threshold  = threshold

	vdoStoreReq := *new(VDOStoreReq)
	vdoStoreReq.Vdo = vdo
	vdoStoreReq.Key =  vdoID // can also use NewRandomID()?

	kadem.Channels.storeVDOIncomingChannel <- vdoStoreReq

	return
}

func (kadem *Kademlia) UnvanishData(vdo VanashingDataObject) (data []byte) {
	//find the original shared key locations
	locationsToStoreKeyIn := CalculateSharedKeyLocations(vdo.AccessKey, int64(vdo.NumberKeys)) // look at second argument
	numberPieces := 0
	mapForCombine := make (map[byte][]byte)

	for _, element := range locationsToStoreKeyIn {
		if int64(numberPieces) >= int64(vdo.Threshold) {break}
		val, err := kadem.DoIterativeFindValue(element)
		fmt.Println("found val", val)
		if err == nil && val!=nil{
			k := val[0]
			v := val[1:]
			mapForCombine[k]=v
			fmt.Println(k,v)
			numberPieces++
		}
	}

	if int64(numberPieces) < int64(vdo.Threshold) {return nil}

	combinedKey := sss.Combine(mapForCombine)

	text := decrypt(combinedKey, vdo.Ciphertext) 

	return text
}
