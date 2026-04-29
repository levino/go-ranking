// Package passphrase generates short adjective+noun pairs.
package passphrase

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
)

var adjectives = []string{
	"jumping", "happy", "silent", "brave", "clever", "swift", "tiny",
	"bright", "calm", "gentle", "lucky", "merry", "noble", "proud",
	"quiet", "rapid", "shiny", "stormy", "wild", "witty", "zesty",
	"breezy", "dreamy", "fancy", "feisty", "fluffy", "fuzzy", "giant",
	"grumpy", "honest", "jolly", "lazy", "mellow", "nimble", "plucky",
	"polite", "rusty", "snappy", "snowy", "sparkly", "spunky", "sunny",
	"tasty", "tender", "toasty", "twinkly", "velvet", "windy", "wooly",
	"zany",
}

var nouns = []string{
	"hippo", "panda", "tiger", "lemur", "otter", "raven", "fox",
	"badger", "beaver", "cobra", "dolphin", "eagle", "falcon", "gecko",
	"heron", "iguana", "jaguar", "koala", "lynx", "moose", "newt",
	"ocelot", "puffin", "quokka", "rabbit", "salmon", "tapir", "urchin",
	"viper", "walrus", "xerus", "yak", "zebra", "antler", "boulder",
	"canyon", "delta", "ember", "fjord", "glacier", "harbor", "island",
	"jungle", "kelp", "lagoon", "meadow", "nebula", "orchard", "prairie",
	"quartz",
}

// New returns a fresh "<adjective>-<noun>" pair from a cryptographic
// random source.
func New() string {
	var buf [4]byte
	if _, err := rand.Read(buf[:]); err != nil {
		panic(err)
	}
	v := binary.BigEndian.Uint32(buf[:])
	return fmt.Sprintf("%s-%s",
		adjectives[int(v>>16)%len(adjectives)],
		nouns[int(v&0xffff)%len(nouns)])
}
