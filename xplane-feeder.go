package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"os"
	"strconv"
	"time"
	"unsafe"

	"github.com/influxdb/influxdb/client"
)

// Reference: http://www.nuclearprojects.com/xplane/xplaneref.html
// or check the Instructions directory of your x-plane installation
type xchr byte
type xint int32
type xdob float64
type xflt float32

type VEH1 struct {
	p              xint
	lat_lon_ele    [3]xdob
	psi_the_phi    [3]xflt
	gear_flap_vect [3]xflt
}

type VEHA struct {
	num_p xint

	lat_lon_ele    [10][3]xdob
	psi_the_phi    [10][3]xflt
	gear_flap_vect [10][3]xflt

	lat_view, lon_view, ele_view xdob
	psi_view, the_view, phi_view xflt
}

type FlightData struct {
	latitude  float64 // lat_lon_ele [0]
	longitude float64 // lat_lon_ele [1]
	altitude  float64 // @TODO: convert to meters lat_lon_ele [2]
	track     float64 // psi_the_phi [0]
}

type ResultRow struct {
	Time interface{}
	Cols []ResultCol
}

type ResultCol struct {
	K string
	V interface{}
}

type InfluxResults []*client.Series

func getInfluxData(flightName string, influxCl *client.Client) InfluxResults {
	query := fmt.Sprintf("select lat, lon, altitude, track from /^flight." + flightName + "/ limit 1")
	var infRes InfluxResults
	var err error

	infRes, err = influxCl.Query(query)
	if err != nil {
		log.Fatalf("Query %s failed!", query)
		os.Exit(1)
	}

	if len(infRes) != 1 {
		log.Printf("No series present?!")
	}

	pointsCount := len(infRes[0].GetPoints())
	var _ = pointsCount
	//log.Printf("Series count: %d\n", pointsCount)
	//log.Printf("DATA: %v", infRes[0].GetPoints());

	return infRes
}

func influxToVEH1(airplaneId int, influxQueryResult InfluxResults) VEH1 {
	var oneFeetInMeters = 0.3048
	var alt = (influxQueryResult[0].Points[0][2].(float64)) * oneFeetInMeters
	var lat = influxQueryResult[0].Points[0][3].(float64)
	var lon = influxQueryResult[0].Points[0][4].(float64)
	var hdg = influxQueryResult[0].Points[0][5].(float64)

	//log.Printf("ALT: %f", alt);
	//log.Printf("LAT: %f", lat);
	//log.Printf("LON: %f", lon);
	//log.Printf("HDG: %f", hdg);
	var veh1 = VEH1{xint(airplaneId), [3]xdob{xdob(lat), xdob(lon), xdob(alt)}, [3]xflt{xflt(hdg), 0.0, 0.0}, [3]xflt{0.0, 0.0, 0.0}}
	if alt > 1000.0 {
		veh1 = VEH1{xint(airplaneId), [3]xdob{xdob(lat), xdob(lon), xdob(alt)}, [3]xflt{xflt(hdg), 0.0, 0.0}, [3]xflt{0.0, 0.0, 0.0}}
	}

	return veh1
}

func writeToXplane(udpcon *net.UDPConn, veh1 VEH1) {
	//fmt.Printf("%+v\n", veh1)

	pavio := unsafe.Pointer(&veh1)
	pavio_arr := *((*[56]byte)(pavio))

	data_send := [61]byte{'V', 'E', 'H', '1', 0}
	n := copy(data_send[5:], pavio_arr[:])
	var _ = n
	//fmt.Printf("Copied %d bytes\n", n)

	//fmt.Printf("%+v\n", data_send)

	buf := new(bytes.Buffer)
	if err := binary.Write(buf, binary.BigEndian, &data_send); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	_, err := udpcon.Write(buf.Bytes())
	checkError(err)
}

func checkError(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "Fatal error ", err.Error())
		os.Exit(1)
	}
}

func main() {

	protocol := "udp"
	host := "127.0.0.1:49000" // x-plane machine

	udpAddr, err := net.ResolveUDPAddr(protocol, host)
	if err != nil {
		fmt.Println("Wrong address!")
		return
	}

	conn, err := net.DialUDP(protocol, nil, udpAddr)
	checkError(err)

	influxClient := &client.ClientConfig{ // influxdb machine
		Host:     "yourserverhostorip:19986",
		Username: "username",
		Password: "password",
		Database: "flightdata",
	}
	fluxClient, err := client.NewClient(influxClient)
	if err != nil {
		log.Fatal(err)
	}
	if len(os.Args) != 3 {
		fmt.Println("Usage: ", os.Args[0], "<ROF123> <AirCraftID>")
		os.Exit(1)
	}
	flightName := os.Args[1]
	if aircraftId, err := strconv.Atoi(os.Args[2]); err != nil {
		panic(err)
	} else {

		for {
			writeToXplane(conn, influxToVEH1(aircraftId, getInfluxData(flightName, fluxClient)))
			// @20Hzish
			time.Sleep(100 * time.Millisecond)
		}
	}

}
