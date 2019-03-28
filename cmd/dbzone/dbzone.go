package main

import (
	"database/sql"
	"flag"
	"fmt"
	"github.com/netsec-ethz/rains/internal/pkg/object"
	"github.com/netsec-ethz/rains/internal/pkg/section"
	"github.com/netsec-ethz/rains/internal/pkg/zonefile"
	"log"
	"strings"

	_ "github.com/go-sql-driver/mysql"
	"github.com/scionproto/scion/go/lib/addr"
)

func main() {

	// usage
	flag.Usage = func() {
		fmt.Println("This tool searches the SCIONLab database for user ASes and creates a zonefile using that data.")
		flag.PrintDefaults()
	}

	// flags
	zName := flag.String("z", "", "zone for which the zonefile is created")
	context := flag.String("c", ".", "context in which assertions are valid")
	out := flag.String("o", "zonefile.txt", "output path")
	prefix := flag.String("p", "", "prefix to use for subject names")

	flag.Parse()

	if *zName == "" {
		log.Fatal("Zone name cannot be empty")
	}

	// Open database
	db, err := sql.Open("mysql", "root:development_pass@/scion_coord_test")
	check(err)
	err = db.Ping()
	check(err)

	rows, err := db.Query(`SELECT isd, as_id, public_ip FROM scion_coord_test.scion_lab_as
	WHERE as_id NOT IN (SELECT as_id FROM scion_coord_test.attachment_point);`)
	check(err)

	// prepare zone
	zone := section.Zone{SubjectZone: *zName, Context: *context}

	for rows.Next() {
		var isd int
		var asID addr.AS
		var ip string
		err = rows.Scan(&isd, &asID, &ip)
		check(err)

		as := asID.String()
		parts := strings.Split(as, ":")
		if len(parts) < 3 {
			log.Fatal("no valid AS identifier")
		}

		obj := object.Object{Type: object.OTScionAddr4, Value: fmt.Sprintf("%d-%s,[%s]", isd, as, ip)}
		name := fmt.Sprintf("%s%s", *prefix, parts[len(parts)-1])
		assertion := section.Assertion{SubjectName: name, Content: []object.Object{obj}}
		zone.Content = append(zone.Content, &assertion)
	}

	consistent := zone.IsConsistent()
	if !consistent {
		log.Fatal("Zone consistency check failed")
	}
	enc := zonefile.IO{}.Encode([]section.Section{&zone})
	sections := []section.Section{&zone}
	zonefile.IO{}.EncodeAndStore(*out, sections)
	fmt.Println(enc)

}

func check(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
