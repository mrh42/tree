package main

import (
	"bytes"
	"github.com/iand/gedcom"
	"io"
	"io/ioutil"
	"fmt"
	"strings"
	"strconv"
	"os"
	"encoding/json"
	"net/http"
)


type Data struct {
	g           *gedcom.Gedcom
	//rootid      string
	//individuals map[string]*gedcom.IndividualRecord
	ixrefs      map[string]int
}

func NewData(filename string) (data *Data) {
	data = &Data{}

	raw, _ := ioutil.ReadFile(filename)
	d := gedcom.NewDecoder(bytes.NewReader(raw))
	data.g, _ = d.Decode()

	//data.individuals = make(map[string]*gedcom.IndividualRecord)
	data.ixrefs = make(map[string]int)
	//data.rootid = data.g.Individual[0].Xref

	for i, rec := range data.g.Individual {
		//data.individuals[rec.Xref] = rec
		data.ixrefs[rec.Xref] = i
	}
	fmt.Printf("Tree contains %d individuals\n", len(data.ixrefs))
	return
}

func (d *Data) idx(xref string) (id int) {
	id, ok := d.ixrefs[xref]
	if !ok {
		id = -1
	}
	return
}
func (d *Data) ind(id int) (*gedcom.IndividualRecord) {
	return d.g.Individual[id]
}

func (d *Data) Name(id int) (n string) {
	i := d.ind(id)
	if i == nil { return }

	n = i.Name[0].Name
	return
}
func (d *Data) Sex(id int) (s string) {
	i := d.ind(id)
	if i == nil { return }

	s = i.Sex
	return
}

type InfoS struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Sex         string `json:"sex"`
	Birth       string `json:"birth"`
	Birthplace  string `json:"birthplace"`
	Death       string `json:"death"`
	Deathplace  string `json:"deathplace"`
	Father      int    `json:"father"`
	Mother      int    `json:"mother"`
	Children    []int  `json:"children"`
	Spouses     []int  `json:"spouses"`
	
}
func (d *Data) Info(id int) (j string) {

	info := &InfoS{ID:id}
	info.Name = d.Name(id)
	info.Sex = d.Sex(id)
	info.Birth, info.Birthplace = d.Event(id, "BIRT")
	info.Death, info.Deathplace = d.Event(id, "DEAT")
	info.Children = d.Children(id)
	info.Spouses = d.Spouses(id)
	info.Mother = d.Mother(id)
	info.Father = d.Father(id)
	jd, _ := json.Marshal(info)
	j = string(jd)
	return
}

func (d *Data) Event(id int, tag string) (date, place string) {
	i := d.ind(id)
	if i == nil { return }

	ev := i.Event
	for _, e := range ev {
		if e.Tag == tag {
			date = e.Date
			place = e.Place.Name
			return
		}
	}
	return
}

func (d *Data) Spouses(id int) (sids []int) {
	sids = make([]int, 0, 10)

	i := d.ind(id)
	if i == nil { return }
	fs := i.Family

	for _, fl := range fs {
		f := fl.Family

		wifeid := d.idx(f.Wife.Xref)
		if wifeid != id {
			sids = append(sids, wifeid)
		}
		husbandid := d.idx(f.Husband.Xref)
		if husbandid != id {
			sids = append(sids, husbandid)
		}
	}
	return
}

func (d *Data) Children(id int) (cids []int) {
	cids = make([]int, 0, 10)

	i := d.ind(id)
	if i == nil { return }
	fs := i.Family

	for _, fl := range fs {
		f := fl.Family
		for _, c := range f.Child {
			cid := d.idx(c.Xref)
			cids = append(cids, cid)
		}
	}
	return
}

func (d *Data) Father(id int) (fid int) {
	fid = -1
	i := d.ind(id)
	if i == nil { return }
	pf := i.Parents
	if len(pf) > 0 {
		f := pf[0].Family
		if f.Husband != nil {
			fid = d.idx(f.Husband.Xref)
		}
	}
	return
}
func (d *Data) Mother(id int) (mid int) {
	mid = -1
	i := d.ind(id)
	if i == nil { return }
	pf := i.Parents
	if len(pf) > 0 {
		f := pf[0].Family
		if f.Wife != nil {
			mid = d.idx(f.Wife.Xref)
		}
	}
	return
}

type ChatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}


func llm(userPrompt string) string {
	apikey := os.Getenv("HUGGING_FACE_HUB_TOKEN")
	//url := "http://100.64.0.9:8000/v1/chat/completions" // Update port if your vLLM differs
	url := "https://router.huggingface.co/v1/chat/completions"

	// Construct the prompt
	systemPrompt := `you have complete access to my family tree. We reference individuals by a unique integer ID.
You can ask for info on individuals by telling me the IDs and I'll provide the info in the next prompt.
We will do this over and over until you have the data you need.
When giving me these IDs, state "NEEDED", then provide as a list, each on its own line with no adornment.
`

	// Build the payload
	payload := map[string]interface{}{
		"model": "google/gemma-4-31B-it",
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": userPrompt},
		},
		// This forces the model to output valid JSON
		//"response_format": map[string]string{"type": "json_object"},
		"temperature":     0.3,
	}

	jsonData, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apikey))

	// Execute the HTTP Request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Println("Error:", err)
		return ""
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	
	var chatResp ChatResponse
	if err := json.Unmarshal(body, &chatResp); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to parse JSON response: %v\nRaw: %s\n", err, string(body))
		return ""
	}
	if chatResp.Error.Message != "" {
		fmt.Fprintf(os.Stderr, "API Error: %s\n", chatResp.Error.Message)
		os.Exit(1)
	}

	if len(chatResp.Choices) == 0 {
		fmt.Fprintf(os.Stderr, "No choices, API Error: %s\n", chatResp.Error.Message)
		return ""
	}
	return chatResp.Choices[0].Message.Content
}

func main() {

	question := os.Args[1]

	d := NewData("mrh-tree.ged")

	ids := make([]int, 0, 100)
	ids = append(ids, 0)
	//ids = append(ids, "I252568536006")
	//ids = append(ids, "I252568535918")
	for {
		
		prompt := fmt.Sprintf("My ID is: %d.  %s\n", 0, question)
	
		for _, id := range(ids) {
			j := d.Info(id)
			prompt += j
		}

		//fmt.Println(prompt)
		resp := llm(prompt)
		fmt.Println("------------------")
		fmt.Println(resp)
		fmt.Println("------------------")

		index := strings.Index(resp, "NEEDED")
		
		if index < 0 {
			break
		} else {
			r := resp[index + 6:]
			needed := strings.Split(r, "\n")
			for _, n := range needed {
				id, err := strconv.Atoi(n)
				if err == nil {
					fmt.Printf("adding info for id: %d, %s\n", id, d.Name(id))
					ids = append(ids, id)
				}
			}
		}
	}
}
