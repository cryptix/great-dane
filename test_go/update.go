// from https://miek.nl/2014/august/16/go-dns-package/
// usage:
//	go run update.go <domain>
// where <domain> is the FQDN for the update
package main

import (
    "github.com/miekg/dns"
    "github.com/davecgh/go-spew/spew"
    "crypto"
    "net"
    "time"
    "os"
    "log"
    "fmt"
)

func main() {

    // TODO: Tidy env var extraction
    log.Printf("-- Collect Environment Variables --\n")
    zone, ok := os.LookupEnv("GD_ZONE")
    if !ok {
        log.Println("No GD_ZONE ENV var defined")
        zone := os.Args[1]
        log.Printf("Using cmdline zone value: %s\n", zone)
    } else {
        log.Printf("GD_ZONE = %s\n", zone)
    }
    host, ok := os.LookupEnv("GD_HOST")
    if !ok {
        log.Println("No GD_HOST ENV var defined")
        host := "go-dns-test"
        log.Printf("Using cmdline host value: %s\n", host)
    } else {
        log.Printf("GD_HOST = %s\n", host)
    }
    server, ok := os.LookupEnv("GD_SERVER")
    if !ok {
        log.Println("No GD_SERVER ENV var defined")
        server := os.Args[2]
        log.Printf("Using cmdline server value: %s\n", server)
    } else {
        log.Printf("GD_SERVER = %s\n", server)
    }

    fqdn := fmt.Sprintf("%s.%s", host, zone)
    log.Printf("Using FQDN RR entry of %s\n", fqdn)

    // TODO make RR generic, for now A record for localhost
    myRR := fmt.Sprintf("%s.%s 600 IN A 127.0.0.1", host, zone)
    log.Printf("myRR = %s\n", myRR)

    log.Println("-- Set dns.Msg Structure --")
    m := new(dns.Msg)
    m.SetUpdate(dns.Fqdn(zone))

    log.Println("-- Attach RR to dns.Msg --")
    rrInsert, err := dns.NewRR(myRR)
    if err != nil {
        panic(err)
    }

    log.Println(spew.Sdump(rrInsert))

    m.Insert([]dns.RR{rrInsert})


    sig0Keyfiles, ok := os.LookupEnv("GD_SIG0_KEYFILES")
    if !ok {
        log.Println("No GD_SIG0_KEYFILES ENV var defined")
    } else {
        log.Println("-- Read SIG(0) Keyfiles --")
        log.Printf("GD_SIG0_KEYFILES = %s", sig0Keyfiles)
        pubfh, perr := os.Open(sig0Keyfiles+".key")
        if perr != nil { log.Fatal(perr) }
        defer pubfh.Close()

        dk, pkerr := dns.ReadRR(pubfh, sig0Keyfiles+".key")
        if pkerr != nil { log.Fatal(pkerr) }

	log.Println(spew.Sdump(dk))

        privfh, oerr := os.Open(sig0Keyfiles+".private")
        if oerr != nil { log.Fatal(oerr) }
        defer privfh.Close()

        privkey, readerr := dk.(*dns.KEY).ReadPrivateKey(privfh, sig0Keyfiles+".private")
        if readerr == nil {
            log.Println(spew.Sdump(privkey))
            log.Println("OK")
        } else {
            log.Println(spew.Sdump(privkey, readerr))
        }
        // create & fill KEY structure (see sig0_test.go for guidance)
        log.Println("-- TODO Create and fill KEY structure from dnssec-keygen keyfiles --")
        keyRR := new(dns.KEY)
        // keyRR.Hdr.Name = dns.AlgorithmToString[uint8(dns.ED25519)] + "." // TODO set to RRset 1st space separated field of dnssec-keygen .key file eg vortex.zenr.io.
        keyRR.Hdr.Name = "vortex.zenr.io." // TODO set to RRset 1st space separated field of dnssec-keygen .key file eg vortex.zenr.io.
	keyRR.Hdr.Rrtype = dns.TypeKEY
	keyRR.Hdr.Class = dns.ClassINET
	keyRR.Hdr.Ttl = 600
	keyRR.Flags = 512
	keyRR.Protocol = 3
	keyRR.Algorithm = uint8(dns.ED25519) // TODO set to numeric alg 6th space separated field of dnssec-keygen .key file eg 15 is ED25519
	keyRR.PublicKey = "2MK3KZkUgYQVumU9bhy1KzIZ2FhFQZ8yLP2nFMJRCEQ=" 

	// Test Generate new ED25519 key
        pk, err := keyRR.Generate(256)
        if err != nil {
                log.Println("failed to generate key: %v", err)
        }
	log.Println(spew.Sdump(pk))

	spew.Dump(keyRR)

        // create & fill SIG structure (see sig0_test.go for guidance)
	log.Println("-- TODO Create, fill & attach SIG RR to dns.Msg Structure --")
	now := uint32(time.Now().Unix())
	sig0RR := new(dns.SIG)
	sig0RR.Hdr.Name = "."
	sig0RR.Hdr.Rrtype = dns.TypeSIG
	sig0RR.Hdr.Class = dns.ClassANY
	sig0RR.Algorithm = uint8(dns.ED25519) // TODO set to alg type of key
	sig0RR.Expiration = now + 300
        sig0RR.Inception = now - 300
        sig0RR.KeyTag = keyRR.KeyTag()
        sig0RR.SignerName = keyRR.Hdr.Name
        mb, err := sig0RR.Sign(pk.(crypto.Signer), m)
        // mb, err := sig0RR.Sign(privkey.(crypto.Signer), m)
	// mb, err := sig0RR.Sign(dk.(crypto.Signer), m)

	if err != nil {
            log.Printf("failed to sign message: %v", err)
        }

	log.Println(spew.Sdump(mb))
	
//	if err := m.Unpack(mb); err != nil {
//          log.Fatalf("failed to unpack message: %v", err)
//      }

	spew.Dump(sig0RR)
    }

    log.Println(spew.Sdump(m))


    log.Println("-- Configure client DNS method --")
    // TODO research how to use config & make sure we directly connect to authoritative server
    config, _ := dns.ClientConfigFromFile("/etc/resolv.conf")
    c := new(dns.Client)

    log.Printf(" ***  Authoritative DNS server (%s) manually selected for message exchange for zone (%s)", server, zone)
    log.Println("-- Send DNS message --")
    r, _, err := c.Exchange(m, net.JoinHostPort(server, config.Port))
    if r == nil {
        log.Fatalf("*** error: %s\n", err.Error())
    }

    if r.Rcode != dns.RcodeSuccess {
    	if r.Rcode == dns.RcodeRefused {
    	        log.Printf(" ***  DNS response refused by server %s for zone (%s)", server, zone)
    	} else {
    	        log.Printf(" ***  DNS response error (%d) from server (%s) for zone (%s)", r.Rcode, server, zone)
    	}
    } else {
        log.Printf(" ***  DNS response from server %s for zone (%s) reports success", server, zone)
    }
    // Stuff must be in the answer section

    log.Printf("-- Answer --")
    for _, a := range r.Answer {
            fmt.Printf("%v\n", a)
    }
    spew.Dump(r)
}

