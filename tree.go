package main

import (
	"bytes"
	"github.com/iand/gedcom"
	"io"
	"io/ioutil"
	"fmt"
	//"strings"
	"regexp"
	"strconv"
	"os"
	"encoding/json"
	"net/http"
	"github.com/lithammer/fuzzysearch/fuzzy"
)


type Data struct {
	g           *gedcom.Gedcom
	ixrefs      map[string]int
	names       []string
}

func NewData(filename string) (data *Data) {
	data = &Data{}

	raw, _ := ioutil.ReadFile(filename)
	d := gedcom.NewDecoder(bytes.NewReader(raw))
	data.g, _ = d.Decode()

	//data.individuals = make(map[string]*gedcom.IndividualRecord)
	data.ixrefs = make(map[string]int)
	data.names = make([]string, len(data.g.Individual))
	//data.rootid = data.g.Individual[0].Xref

	for i, rec := range data.g.Individual {
		//data.individuals[rec.Xref] = rec
		data.ixrefs[rec.Xref] = i
		data.names[i] = rec.Name[0].Name
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
	if id < 0 {return nil}
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

type InfoDetail struct {
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

	info := &InfoDetail{ID:id}
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
type InfoC struct {
	ID          int    `json:"id"`
	Children    []int  `json:"children"`
}

func (d *Data) ChildrenInfo(id int) (j string) {
	info := &InfoC{ID:id}
	info.Children = d.Children(id)
	jd, _ := json.Marshal(info)
	j = string(jd)
	return
}
type InfoP struct {
	ID          int    `json:"id"`
	Father      int    `json:"father"`
	Mother      int    `json:"mother"`
}

func (d *Data) ParentInfo(id int) (j string) {
	info := &InfoP{ID:id}
	info.Mother = d.Mother(id)
	info.Father = d.Father(id)
	jd, _ := json.Marshal(info)
	j = string(jd)
	return
}
type InfoS struct {
	ID          int    `json:"id"`
	Spouses     []int  `json:"spouses"`
}

func (d *Data) SpouseInfo(id int) (j string) {
	info := &InfoS{ID:id}
	info.Spouses = d.Spouses(id)
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
		if f == nil {continue}

		if f.Wife != nil {
			wifeid := d.idx(f.Wife.Xref)
			if wifeid >= 0 && wifeid != id {
				sids = append(sids, wifeid)
			}
		}
		if f.Husband != nil {
			husbandid := d.idx(f.Husband.Xref)
			if husbandid >= 0 && husbandid != id {
				sids = append(sids, husbandid)
			}
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
			//fmt.Printf("cid: %d x: %s\n", cid, c.Xref)
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

func (d *Data) Search(name string) (ids map[int]bool) {

	ids = make(map[int]bool)
	name, _ = strconv.Unquote(name)

	matches := fuzzy.RankFind(name, d.names)
	for i, m := range matches {
		ids[m.OriginalIndex] = true
		//fmt.Printf("matches: %v\n", m)
		if i > 20 {
			break
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

const xxfunctionPrompt = `you have complete access to my family tree. We reference individuals by a unique integer ID.
You can ask for info on individuals by telling me the IDs and I'll provide the info in the next prompt.
In your response use "PARENTS(id)" to find an indivuals mother and fater and "CHILDREN(id)" to list an individual's children and "SPOUSES(id)" to list spouses and INFO(id) for more details on the individual.
We will do this over and over until you have the data you need.
`
const xfunctionPrompt = `you have complete access to my family tree. You reference individuals by a unique integer ID when needing information.
You must ask for info on individuals by telling me the IDs and I'll provide the info in the next prompt.
In your response you must invoke INFO(id) for each individual id you require details for.  Do not ever write INFO() unless needing new information.
We will do this over and over until you have the data you need. Avoid using markdown.`

const functionPrompt = `you have complete access to my family tree. You reference individuals by a unique integer ID when needing information.
You must ask for info on individuals by telling me the IDs and I'll provide the info in the next prompt.
In your response you must invoke INFO(id) for each individual id you require details for.
You can lookup IDs for people by name with SEARCH("name"), only use when needed, remember to use quotes.
We will do this over and over until you have the data you need. Avoid using markdown.`


const finalPrompt = `Provide a detailed response in plain text, avoid markdown. Here is structured information from my family tree. We reference individuals by a unique integer ID.  When the user asks generically about a person, provide their name and dates of birth and death.`


func llm(userPrompt string, data string, final bool) string {
	apikey := os.Getenv("HUGGING_FACE_HUB_TOKEN")
	//url := "http://100.64.0.9:11434/v1/chat/completions"
	url := "https://router.huggingface.co/v1/chat/completions"

	//model := "google/gemma-4-26B-A4B-it"
	model := "google/gemma-4-31B-it"

	temp := 0.3
	systemPrompt := functionPrompt
	if final {
		systemPrompt = finalPrompt
		temp = 0.7
	}	
	userPrompt += " here is the data: " + data

	// Build the payload
	payload := map[string]interface{}{
		"model": model, 
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": userPrompt},
		},
		// This forces the model to output valid JSON
		//"response_format": map[string]string{"type": "json_object"},
		"temperature":     temp,
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

	ids := make(map[int]bool)
	cids := make(map[int]bool)
	pids := make(map[int]bool)
	sids := make(map[int]bool)

	ids[0] = true
	for {
		
		prompt := fmt.Sprintf("My ID is: %d.  %s\n", 0, question)
		data := ""
		
		for id := range(ids) {
			j := d.Info(id)
			data += j
		}
		for id := range(cids) {
			j := d.ChildrenInfo(id)
			data += j
		}
		for id := range(pids) {
			j := d.ParentInfo(id)
			data += j
		}
		for id := range(sids) {
			j := d.SpouseInfo(id)
			data += j
		}

		//fmt.Println(data)
		resp := llm(prompt, data, false)
		fmt.Println("------------------")
		fmt.Println(resp)
		fmt.Println("------------------")

		num_ids := len(ids)
		//re := regexp.MustCompile(`([A-Z_][A-Z0-9_]*)\((\d+)\)`)
		re := regexp.MustCompile(`([A-Z_][A-Z0-9_]*)\((.+)\)`)

		matches := re.FindAllStringSubmatch(resp, -1)
		for _, m := range matches {
			if m[1] == "SEARCH" {
				searched := d.Search(m[2])
				for id := range searched {
					ids[id] = true
				}
			}
			id, err := strconv.Atoi(m[2])
			if err == nil {
				f := m[1]
				//fmt.Printf("call %s %d\n", f, id)
				if f == "INFO" {
					ids[id] = true
				}
				if f == "PARENTS" {
					pids[id] = true
				}
				if f == "CHILDREN" {
					cids[id] = true
				}
				if f == "SPOUSES" {
					sids[id] = true
				}
			}
		}
		if len(ids) == num_ids {
			resp := llm(prompt, data, true)
			fmt.Println("---- no progress made --------------")
			fmt.Println(resp)
			break
		}
	}
}
