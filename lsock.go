// Copyright 2014 Lieven Govaerts. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"github.com/aschepis/kernctl"
	"io"
)

const (
	// Providers
	NSTAT_PROVIDER_ROUTE = 1
	NSTAT_PROVIDER_TCP   = 2
	NSTAT_PROVIDER_UDP   = 3

	// generic response messages
	NSTAT_MSG_TYPE_SUCCESS = 0
	NSTAT_MSG_TYPE_ERROR   = 1

	// Requests
	NSTAT_MSG_TYPE_ADD_SRC      = 1001
	NSTAT_MSG_TYPE_ADD_ALL_SRCS = 1002
	NSTAT_MSG_TYPE_REM_SRC      = 1003
	NSTAT_MSG_TYPE_QUERY_SRC    = 1004
	NSTAT_MSG_TYPE_GET_SRC_DESC = 1005

	// Responses/Notfications
	NSTAT_MSG_TYPE_SRC_ADDED   = 10001
	NSTAT_MSG_TYPE_SRC_REMOVED = 10002
	NSTAT_MSG_TYPE_SRC_DESC    = 10003
	NSTAT_MSG_TYPE_SRC_COUNTS  = 10004

	NSTAT_SRC_REF_ALL     = 0xFFFFFFFF
	NSTAT_SRC_REF_INVALID = 0
)

type nstat_msg_hdr struct {
	Context uint64
	HType   uint32
	Pad     uint32 // unused for now
}

/*****************************************************************************/
/* REQUESTS                                                                  */
/*****************************************************************************/

// Type nstat_msg_add_all_srcs, implements kernctl.Message for serialization.
type nstat_msg_add_all_srcs struct {
	Hdr      nstat_msg_hdr
	Provider uint32
}

func (msg *nstat_msg_add_all_srcs) Bytes() []byte {
	buf := new(bytes.Buffer)
	binary.Write(buf, binary.LittleEndian, msg)
	return buf.Bytes()
}

type nstat_msg_query_src_req struct {
	Hdr    nstat_msg_hdr
	SrcRef uint32
}

func (msg *nstat_msg_query_src_req) Bytes() []byte {
	buf := new(bytes.Buffer)
	binary.Write(buf, binary.LittleEndian, msg)
	return buf.Bytes()
}

type nstat_msg_get_src_description struct {
	Hdr    nstat_msg_hdr
	SrcRef uint32
}

func addAllSources(conn *kernctl.Conn, provider uint32) {
	aasreq := &nstat_msg_add_all_srcs{
		Hdr: nstat_msg_hdr{
			HType:   NSTAT_MSG_TYPE_ADD_ALL_SRCS,
			Context: 3,
		},
		Provider: provider,
	}
	fmt.Println("addAllSources", provider)
	conn.SendCommand(aasreq)
}

func getCounts(conn *kernctl.Conn, srcRef uint32) {
	qsreq := &nstat_msg_query_src_req{
		Hdr: nstat_msg_hdr{
			HType:   NSTAT_MSG_TYPE_QUERY_SRC,
			Context: 1005,
		},
		SrcRef: srcRef,
	}
	fmt.Println("getCounts", srcRef)
	conn.SendCommand(qsreq)
}

/*****************************************************************************/
/* RESPONSES                                                                 */
/*****************************************************************************/
type nstat_msg_src_added struct {
	Hdr      nstat_msg_hdr
	Provider uint32
	SrcRef   uint32
}

type nstat_msg_src_removed struct {
	Hdr    nstat_msg_hdr
	SrcRef uint32
}

type nstat_msg_src_description struct {
	Hdr      nstat_msg_hdr
	SrcRef   uint32
	Provider uint32
	// u_int8_t  data[];
}

// The original C structures are #pragma pack(1), but here this isn't important,
// we let binary.Read map packed byte stream to unpacked go struct.
type nstat_counts struct {
	Rxpackets         uint64
	Rxbytes           uint64
	Txpackets         uint64
	Txbytes           uint64
	Rxduplicatebytes  uint32
	Rxoutoforderbytes uint32
	Txretransmit      uint32
	Connectattempts   uint32
	Connectsuccesses  uint32
	Min_rtt           uint32
	Avg_rtt           uint32
	Var_rtt           uint32
}

type nstat_msg_src_counts struct {
	Hdr    nstat_msg_hdr
	SrcRef uint32
	Counts nstat_counts
}

const (
	AF_INET  = 2
	AF_INET6 = 30
)

type in6_addr struct {
	S6_addr [16]uint8
}

type sockaddr_in6 struct {
	Sin6_len      uint8
	Sin6_family   uint8
	Sin6_port     uint16
	Sin6_flowinfo uint32
	Sin6_addr     [16]uint8
	Sin6_scope_id uint32
}

/* From netinet/in.h:
   struct sockaddr_in {
       __uint8_t       sin_len;
       sa_family_t     sin_family;
       in_port_t       sin_port;
       struct  in_addr sin_addr;
       char            sin_zero[8];
   };
*/
type sockaddr_in4 struct {
	Sin_len    uint8
	Sin_family uint8
	Sin_port   uint16
	Sin_addr   [4]uint8
	Sin_zero   [8]byte
}

type nstat_tcp_descriptor struct {
	/*	short            sin_family   // e.g. AF_INET
		unsigned short   sin_port    // e.g. htons(3490)
		struct in_addr   sin_addr     // see struct in_addr, below
		char             sin_zero[8]  // zero this if you want to
	*/
	/*        union
	          {
	                  struct sockaddr_in      v4
	                  struct sockaddr_in6     v6
	          } local

	          union
	          {
	                  struct sockaddr_in      v4
	                  struct sockaddr_in6     v6
	          } remote
	*/
	Local      [28]byte
	Remote     [28]byte
	Ifindex    uint32
	State      uint32
	Sndbufsize uint32
	Sndbufused uint32
	Rcvbufsize uint32
	Rcvbufused uint32
	Txunacked  uint32
	Txwindow   uint32
	Txcwindow  uint32
	Upid       uint64
	Pid        uint32
	Pname      [64]uint8
}

// Read the sockaddr structures in network byte order!
func (d *nstat_tcp_descriptor) Local4() (sockaddr_in4, error) {
	var addr sockaddr_in4
	var tmp []byte
	tmp = d.Local[0:16]
	reader := bytes.NewReader(tmp)
	err := binary.Read(reader, binary.BigEndian, &addr)
	return addr, err
}

func (d *nstat_tcp_descriptor) Remote4() (sockaddr_in4, error) {
	var addr sockaddr_in4
	var tmp []byte
	tmp = d.Remote[0:16]
	reader := bytes.NewReader(tmp)
	err := binary.Read(reader, binary.BigEndian, &addr)
	return addr, err
}

func (d *nstat_tcp_descriptor) Family() uint8 {
	return uint8(d.Local[1])
}

type Descriptor struct {
	zzz    string
	Counts nstat_counts
}

var descriptors map[uint32]*Descriptor

/*****************************************************************************/

func readTCPDescriptor(msg nstat_msg_src_description, reader io.Reader) error {
	var tcpDesc nstat_tcp_descriptor

	// Read the remainder of the data in the nstat_tcp_descriptor struct
	err := binary.Read(reader, binary.LittleEndian, &tcpDesc)
	if err != nil {
		fmt.Println("binary.Read TCPDescriptor failed:", err)
		return err
	}
	fmt.Println("tcp descriptor received:", tcpDesc)

	fmt.Println("Family: ", tcpDesc.Family())
	switch tcpDesc.Family() {
	case AF_INET:
		var laddr, raddr sockaddr_in4
		if laddr, err = tcpDesc.Local4(); err != nil {
			break
		}
		if raddr, err = tcpDesc.Remote4(); err != nil {
			break
		}
		fmt.Println("local: ", laddr, " remote: ", raddr)
	case AF_INET6:
		break
	}

	return err
}

/* Process the response we received from the system socket. */
func process_nstat_msg(msg_hdr nstat_msg_hdr, buf []byte) error {

	switch msg_hdr.HType {
	case NSTAT_MSG_TYPE_SRC_ADDED:
		var msg nstat_msg_src_added
		reader := bytes.NewReader(buf)
		err := binary.Read(reader, binary.LittleEndian, &msg)
		if err != nil {
			fmt.Println("binary.Read SRC_ADDED failed:", err)
			break
		}
		fmt.Println("new source: ", msg)
		descriptors[msg.SrcRef] = &Descriptor{}

	case NSTAT_MSG_TYPE_SRC_REMOVED:
		var msg nstat_msg_src_removed
		reader := bytes.NewReader(buf)
		err := binary.Read(reader, binary.LittleEndian, &msg)
		if err != nil {
			fmt.Println("binary.Read SRC_REMOVED failed:", err)
			break
		}
		fmt.Println("source removed: ", msg)
		delete(descriptors, msg.SrcRef)

	case NSTAT_MSG_TYPE_SRC_DESC:
		var msg nstat_msg_src_description
		reader := bytes.NewReader(buf)
		err := binary.Read(reader, binary.LittleEndian, &msg)
		if err != nil {
			fmt.Println("binary.Read SRC_DESCRIPTION failed:", err)
			break
		}
		switch msg.Provider {
		case NSTAT_PROVIDER_TCP:
			fmt.Println("buf: ", buf)
			readTCPDescriptor(msg, reader)
			fmt.Println("TCP descriptor received: ", msg)
		case NSTAT_PROVIDER_UDP:
			fmt.Println("UDP descriptor received: ", msg)
		}
		fmt.Println("description received: ", msg)

	case NSTAT_MSG_TYPE_SRC_COUNTS:
		var msg nstat_msg_src_counts
		reader := bytes.NewReader(buf)
		err := binary.Read(reader, binary.LittleEndian, &msg)
		if err != nil {
			fmt.Println("binary.Read SRC_COUNTS failed:", err)
			break
		}
		fmt.Println("counts received: ", msg)
		descriptors[msg.SrcRef].Counts = msg.Counts

	}
	return nil
}

const (
	STATE_INITIAL      = 0
	STATE_TCP_ADDED    = 2
	STATE_UDP_ADDED    = 4
	STATE_COUNTS_ADDED = 6
)

func main() {
	conn := kernctl.NewConnByName("com.apple.network.statistics")
	if err := conn.Connect(); err != nil {
		panic(err)
	}

	descriptors = make(map[uint32]*Descriptor)

	var state = STATE_INITIAL
	for {
		// Subscribe to following events one by one:
		// 1. all TCP events
		// 2. all UDP events
		// 3. counts
		// 4. descriptions
		switch state {
		case STATE_INITIAL:
			addAllSources(conn, NSTAT_PROVIDER_TCP)
			state++
		case STATE_TCP_ADDED:
			addAllSources(conn, NSTAT_PROVIDER_UDP)
			state++
		case STATE_UDP_ADDED:
			getCounts(conn, NSTAT_SRC_REF_ALL)
			state++
		default:
			/* in one of the waiting states (uneven numbers) */
			break
		}

		if err, buf := conn.Select(2048); err != nil {
			panic(err)
		} else {
			var msg_hdr nstat_msg_hdr

			// we received a message. first read the header, based on the
			// HType field we can the decide how to interpret the complete
			// byte stream.
			reader := bytes.NewReader(buf)
			err := binary.Read(reader, binary.LittleEndian, &msg_hdr)
			if err != nil {
				fmt.Println("binary.Read failed:", err)
				//				break
				continue
			}
			fmt.Println("msg_hdr recvd:", msg_hdr)

			switch msg_hdr.HType {
			case NSTAT_MSG_TYPE_SUCCESS:
				{
					/* Previous requested action was successful, go to next. */
					state++
					fmt.Println("state: ", state, "success context ", msg_hdr.Context)
				}
			case NSTAT_MSG_TYPE_SRC_ADDED, NSTAT_MSG_TYPE_SRC_REMOVED,
				NSTAT_MSG_TYPE_SRC_DESC, NSTAT_MSG_TYPE_SRC_COUNTS:
				{
					ret := process_nstat_msg(msg_hdr, buf)
					if ret != nil {
						break
					}
				}
			case NSTAT_MSG_TYPE_ERROR:
				fmt.Println("error")
			}
		}
	}

	conn.Close()
}
