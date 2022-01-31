package main

import (
    "net"
    "math/rand"
    "encoding/binary"
    "strings"
    "C"
    "fmt"
    "strconv"
)

/**
 * Reminder on the types found in package 'net'
 * IPNet is a struct containing IP IP and Mask IPMask
 * IP is type []byte
 * IPMask is type []byte
 */
const (
    IPv4PrefixLen = 8 * net.IPv4len // max prefix length in bits (32)
)

/**
 * Given a net.IPNet and a mask length, returns a slice containing all subnets of length 'mask length' contained in 'subnet'.
 * ex: 118.174.128.0/22, with mask length 24, gives:
 * - 118.174.128.0/24
 * - 118.174.129.0/24
 * - 118.174.130.0/24
 * - 118.174.131.0/24
 * When 'mask length' is inferior to the mask length of 'subnet', returns the subnet with the new mask length.
 * ex: 118.174.128.0/26, with mask length 24, gives 118.174.128.0/24
 */
func get_subnets (subnet *net.IPNet, mask_length int) []net.IPNet{
    l,_ := subnet.Mask.Size ()
    diff := mask_length - l

    var subnets []net.IPNet
    /* --- Requested mask length inferior to given subnet --- */
    if diff <= 0 {
        subnets = make ([]net.IPNet, 1)

        m := net.CIDRMask(mask_length, IPv4PrefixLen) 
        subnets[0] = *subnet
        subnets[0].IP = subnet.IP.Mask (m) //returns the result of masking the IP address ip with mask
        subnets[0].Mask = m
        
    } else { /* --- Requested mask length superior to given subnet --- */
        nb_subnets := 1<<uint(diff)
        subnets = make ([]net.IPNet, nb_subnets)

        ip := ip_to_uint32 (&subnet.IP) 
        host_length := IPv4PrefixLen - mask_length

        for i := 0; i < nb_subnets; i++ {
            new_ip := ip | uint32 (i << host_length)

            subnets[i] = *new (net.IPNet) // * is to get the value
            subnets[i].IP = *uint32_to_ip (new_ip)
            subnets[i].Mask = net.CIDRMask(mask_length, IPv4PrefixLen)
        }

    }
    return subnets
}

//export get_subnets_string
func get_subnets_string (subnet string, mask_length int, p **C.char){
    _, network, err := net.ParseCIDR (subnet)
        if err != nil {
            *p = C.CString("")
        }
    subnets := get_subnets (network, mask_length)

    // Concatenate all subnets, separated by a dash
    var subnets_string strings.Builder
    sep := ""
    for _, subnet := range subnets {
        subnets_string.WriteString (sep + subnet.String ())
        sep = "-"
    }
    *p = C.CString(subnets_string.String ())
}

//export test_return_string
func test_return_string (p **C.char) {
    *p = C.CString("Hello from go!")
}

func uint32_to_ip (ip uint32) *net.IP {
    b := make ([]byte, net.IPv4len)
    binary.BigEndian.PutUint32(b, ip)
    n := net.IP (b)
    return &n
}

func ip_to_uint32(ip *net.IP) (ret uint32) {
    return uint32(binary.BigEndian.Uint32(ip.To4()))
}

func string_to_ip (prefix string) *net.IP {
    ip, _, err := net.ParseCIDR (prefix) //network is *IPNet, //ip is IP
    if err != nil {
        panic ("[string_to_ip]: Error while parsing prefix: " + err.Error ())
    }
    return &ip
}

func string_to_net (prefix string) *net.IPNet {
    _, network, err := net.ParseCIDR (prefix) //network is *IPNet, //ip is IP
    if err != nil {
        panic ("[string_to_net]: Error while parsing prefix: " + err.Error ())
    }
    return network
}

/**
 * Given a subnet, returns a routable address picked randomly into the subnet.
 * Yields only routable addresses (no host address or network address)
 */
func get_random_ip (subnet *net.IPNet) *net.IP {
    mask_length,_ := subnet.Mask.Size ()
    host_length := IPv4PrefixLen - mask_length

    min := 1 // Host address
    max := (1 << uint(host_length)) - 2 // Network address
    n := rand.Intn(max - min + 1) + min

    ip := ip_to_uint32 (&subnet.IP) 
    ip = ip | uint32 (n)

    return uint32_to_ip (ip)
}

/**
 * Returns the prefix as a binary string.
 * The binary string is cut at mask length.
 * ex: 1.0.4.0/22 -> "0000000100000000000001"
 */
func get_binary_string (prefix string) string {

    ip := strings.Split (prefix, "/")[0]
    ip_byte := net.ParseIP (ip)

    var ip_string string
    if len (ip_byte) == 4 {
        ip_string = fmt.Sprintf("%08b%08b%08b%08b", ip_byte[0], ip_byte[1], ip_byte[2], ip_byte[3])
    } else {
        ip_string = fmt.Sprintf("%08b%08b%08b%08b", ip_byte[12], ip_byte[13], ip_byte[14], ip_byte[15])
    }
    
    l,_ := strconv.Atoi (strings.Split (prefix, "/")[1])
    return ip_string[:l]
}

/**
 * Given a probe under the form x.x.x.x/y, picks a random /24 prefix in it.
 */
func _get_24_prefix (probe string) string {
    if strings.HasSuffix (probe, "/24") {
        return probe
    }
    _, network, _ := net.ParseCIDR (probe)
    ip_address := get_random_ip (network).String ()
    prefix_24 := strings.Join (strings.Split (ip_address, ".")[:3], ".")+".0/24"
    return prefix_24
}

func _get_raw_prefix (probe string) string {
    return probe
}

/**
 * Does the reverse operation of get_binary_string
 */
func get_prefix_from_binary (binary string) string {
    mask := len (binary)
    rest := get_0_string (IPv4PrefixLen - mask)
    binary += rest

    r := ""
    for start := 0; start <= 24; start += 8 {
        c,_ := strconv.ParseUint(binary[start:start+8], 2, 8)
        r += strconv.Itoa (int (c)) + "."
    }
    return r[:len(r)-1] + "/" + strconv.Itoa (mask)
}

func get_0_string (n int) string {
    b := make([]rune, n)
  for i := range b {
      b[i] = '0'
  }
  return string(b)
}

// For building shared library
//func main(){}
