package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"github.com/aschepis/kernctl"
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

/*****************************************************************************/

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

/* Process the response we received from the system socket. */
func process_nstat_msg(msg_hdr nstat_msg_hdr, buf []byte) error {

	switch msg_hdr.HType {
	case NSTAT_MSG_TYPE_SRC_ADDED:
		var msg nstat_msg_src_added
		reader := bytes.NewReader(buf)
		err := binary.Read(reader, binary.LittleEndian, &msg)
		if err != nil {
			fmt.Println("binary.Read failed:", err)
			break
		}
		fmt.Println("new source: ", msg)

	case NSTAT_MSG_TYPE_SRC_REMOVED:
		var msg nstat_msg_src_removed
		reader := bytes.NewReader(buf)
		err := binary.Read(reader, binary.LittleEndian, &msg)
		if err != nil {
			fmt.Println("binary.Read failed:", err)
			break
		}
		fmt.Println("source removed: ", msg)

	case NSTAT_MSG_TYPE_SRC_DESC:
		var msg nstat_msg_src_description
		reader := bytes.NewReader(buf)
		err := binary.Read(reader, binary.LittleEndian, &msg)
		if err != nil {
			fmt.Println("binary.Read src_description failed:", err)
			break
		}
		switch msg.Provider {
		case NSTAT_PROVIDER_TCP:
			fmt.Println("TCP description received: ", msg)
		case NSTAT_PROVIDER_UDP:
			fmt.Println("UDP description received: ", msg)
		}
		fmt.Println("description received: ", msg)

	case NSTAT_MSG_TYPE_SRC_COUNTS:
		var msg nstat_msg_src_counts
		reader := bytes.NewReader(buf)
		err := binary.Read(reader, binary.LittleEndian, &msg)
		if err != nil {
			fmt.Println("binary.Read failed:", err)
			break
		}
		fmt.Println("counts received: ", msg)

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

	var state = STATE_INITIAL
	for {
		// Subscribe to following events one by one:
		// 1. all TCP events
		// 2. all UDP events
		// 3. XXXXXXX
		//
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

			reader := bytes.NewReader(buf)
			err := binary.Read(reader, binary.LittleEndian, &msg_hdr)
			if err != nil {
				fmt.Println("binary.Read failed:", err)
				break
			}
			fmt.Println("msg_hdr recvd:", msg_hdr)

			switch msg_hdr.HType {
			case NSTAT_MSG_TYPE_SUCCESS:
				{
					/* Previous requested action was successful, go to next. */
					state++
				}
			case NSTAT_MSG_TYPE_SRC_ADDED, NSTAT_MSG_TYPE_SRC_REMOVED,
				NSTAT_MSG_TYPE_SRC_DESC, NSTAT_MSG_TYPE_SRC_COUNTS:
				{
					ret := process_nstat_msg(msg_hdr, buf)
					if ret != nil {
						break
					}
				}
			}

		}
	}

	conn.Close()
}
