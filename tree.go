package main

import (
	"bytes"
	"github.com/iand/gedcom"
	"io"
	"io/ioutil"
	"fmt"
	"strings"
	"regexp"
	"strconv"
	"os"
	"flag"
	"time"
	"encoding/json"
	"net/http"
	"github.com/lithammer/fuzzysearch/fuzzy"
)


type Data struct {
	g           *gedcom.Gedcom
	ixrefs      map[string]int
	names       []string
	births      []string
	deaths      []string
	raw         []byte
}

func NewData(filename string) (data *Data) {
	data = &Data{}

	data.raw, _ = ioutil.ReadFile(filename)
	d := gedcom.NewDecoder(bytes.NewReader(data.raw))
	data.g, _ = d.Decode()

	data.ixrefs = make(map[string]int)
	// allows searching names
	data.names = make([]string, len(data.g.Individual))

	for i, rec := range data.g.Individual {
		data.ixrefs[rec.Xref] = i
		//fmt.Printf("%d: %s\n", i, rec.Xref)
		data.names[i] = rec.Name[0].Name
	}

	// allow searching birth and death places
	data.births = make([]string, len(data.names))
	data.deaths = make([]string, len(data.names))
	for i := range data.names {
		_, data.births[i] = data.Event(i, "BIRT")
		_, data.deaths[i] = data.Event(i, "DEAT")
	}
	fmt.Printf("Tree contains %d individuals\n", len(data.ixrefs))
	return
}

// extreme hack for now...
func (d *Data) RawData(xref1 string) (r string) {
	startm := fmt.Sprintf("0 @%s@ INDI", xref1)

	raws := string(d.raw)
	i := strings.Index(raws, startm)

	raws = raws[i:]
	i = strings.Index(raws, "\n0 @")
	r = raws[:i]

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

type Spouse struct {
	ID          int    `json:"id"`
	Married     string `json:"married"`
	Divorced    string `json:"divorced"`
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
	Spouses     []Spouse  `json:"spouses"`
	
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
	j = string(jd) + "\n"
	return
}


type InfoG struct {
	ID          int    `json:"id"`
	GED         string  `json:"ged"`
}

func (d *Data) GEDCOMInfo(id int) (j string) {
	ged := &InfoG{ID:id}

	i := d.ind(id)
	if i != nil {
		ged.GED = d.RawData(i.Xref)
	}
	jd, _ := json.Marshal(ged)
	j = string(jd) + "\n"
	return
}


func (d *Data) Event(id int, tag string) (date, place string) {
	i := d.ind(id)
	if i == nil { return }

	ev := i.Event
	for _, e := range ev {
		//fmt.Printf("event: id: %d tag: %s type: %s date: %s place: %s\n", id, e.Tag, e.Type, e.Date, e.Place.Name)
		if e.Tag == tag {
			date = e.Date
			place = e.Place.Name
			return
		}
	}
	return
}

func (d *Data) Spouses(id int) (spouses []Spouse) {
	spouses = make([]Spouse, 0, 10)

	i := d.ind(id)
	if i == nil { return }
	fs := i.Family

	for _, fl := range fs {
		f := fl.Family
		if f == nil {continue}

		var spouse Spouse
		for _, fe := range f.Event {
			//fmt.Printf("id: %d, tag: %s date: %s place: %s\n", id, fe.Tag, fe.Date, fe.Place)
			if fe.Tag == "MARR" {
				spouse.Married = fmt.Sprintf("%s %s", fe.Date, fe.Place.Name)
			}
			if fe.Tag == "DIV" {
				spouse.Divorced = fmt.Sprintf("%s %s", fe.Date, fe.Place.Name)
			}
		}

		if f.Wife != nil {
			wifeid := d.idx(f.Wife.Xref)
			if wifeid >= 0 && wifeid != id {
				spouse.ID = wifeid
				spouses = append(spouses, spouse)
			}
		}
		if f.Husband != nil {
			husbandid := d.idx(f.Husband.Xref)
			if husbandid >= 0 && husbandid != id {
				spouse.ID = husbandid
				spouses = append(spouses, spouse)
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

// only returns one father, but good enough for demo
func (d *Data) Father(id int) (fid int) {
	fid = -1
	i := d.ind(id)
	if i == nil { return }
	pf := i.Parents
	for _, p := range pf {
		f := p.Family
		if f.Husband != nil {
			fid = d.idx(f.Husband.Xref)
			if fid >= 0 {
				break
			}
		}
	}
	return
}

// only returns one mother, but good enough for demo
func (d *Data) Mother(id int) (mid int) {
	mid = -1
	i := d.ind(id)
	if i == nil { return }
	pf := i.Parents
	for _, p := range pf {
		f := p.Family
		if f.Wife != nil {
			mid = d.idx(f.Wife.Xref)
			if mid >= 0 {
				break
			}
		}
	}
	return
}

// quick hack to let the llm search
func (d *Data) Search(field, name string) (ids map[int]bool) {

	ids = make(map[int]bool)
	//name, _ = strconv.Unquote(name)

	var target []string
	switch field {
	case "BIRTH":
		target = d.births
	case "DEATH":
		target = d.deaths
	default:
		target = d.names
	}
	matches := fuzzy.RankFind(name, target)
	for i, m := range matches {
		ids[m.OriginalIndex] = true

		// limit how many people we will add
		if i > 200 {
			break
		}
	}
	fmt.Printf("Search(%s) added %d id entries\n", name, len(ids))
	return
}

type LLM struct {
	model     string
	url       string

	systemPrompt string
	temp      float32
	elapsed   time.Duration
}

func NewLLM(model, url string) (llm *LLM) {
	llm = &LLM{}

	llm.model = model
	llm.url = url
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


func (llm *LLM) Chat(userPrompt string, data string) (string, error) {
	apikey := os.Getenv("HUGGING_FACE_HUB_TOKEN")

	userPrompt += " here is the data: " + data

	// Build the payload
	payload := map[string]interface{}{
		"model": llm.model, 
		"messages": []map[string]string{
			{"role": "system", "content": llm.systemPrompt},
			{"role": "user", "content": userPrompt},
		},
		// This forces the model to output valid JSON
		//"response_format": map[string]string{"type": "json_object"},
		"temperature":     llm.temp,
	}

	startt := time.Now()

	jsonData, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", llm.url, bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apikey))

	// Execute the HTTP Request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Println("Error:", err)
		return "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	llm.elapsed = time.Now().Sub(startt)
	
	var chatResp ChatResponse
	if err := json.Unmarshal(body, &chatResp); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to parse JSON response: %v\nRaw: %s\n", err, string(body))
		return "", err
	}
	if chatResp.Error.Message != "" {
		fmt.Fprintf(os.Stderr, "API Error: %s\n", chatResp.Error.Message)
		return "", nil
	}

	if len(chatResp.Choices) == 0 {
		fmt.Fprintf(os.Stderr, "No choices, API Error: %s\n", chatResp.Error.Message)
		return "", nil
	}
	return chatResp.Choices[0].Message.Content, nil
}

const functionPrompt = `
You are an analytical assistant with complete access to my family tree database. Your goal is to answer user queries by iteratively searching for and retrieving information. 

Individuals in the database are referenced by a unique integer ID. You do not have the data yet; you must retrieve it using the commands below.

AVAILABLE DATABASE COMMANDS:
To query the database, output the following commands. Each command MUST be on a new line with no other text.
INFO id (Returns detailed info for the individual's ID)
GEDCOM id (Returns raw GEDCOM data. Use ONLY if the user explicitly requests it, as it is very large)
SEARCH name (Looks up IDs for people by name)
BIRTH place (Looks up IDs for people by birth location)
DEATH place (Looks up IDs for people by death location)

STATE MANAGEMENT COMMANDS, use these to improve performance of the next round:
HINT text (Passes a thought or note to your next prompt round)
REMEMBER text (Adds a permanent fact to your knowledge for all future rounds)

for example:
INFO 42
SEARCH Tom Jones
HINT 42 is the son of 57
REMEMBER the father of 33 is 37

STRICT RULES:
1. Output plain text ONLY. Do not use any markdown, bolding, asterisks, or code blocks.
2. Each command must be on a separate line. You can issue multiple commands in a single response.
3. CRITICAL: Once you issue database commands, you must WAIT. Do not attempt to answer the user's final question until I provide the data back to you in the next prompt.
4. We will repeat this loop as many times as necessary until you have enough data to form a final answer.
5. In the final answer, you will need names and other infomation for the individuals involved, not just the ID numbers.
`



const xxfinalPrompt = `
Answer the user's question using the supplied family-tree data.

Include only information that directly helps answer the question.
State the main conclusion first, followed by the supporting genealogical facts.
Identify uncertainty or conflicting information explicitly.
Do not discuss your process, the supplied data, or information that is unrelated to the question.

Use concise paragraphs. Do not attempt to produce a comprehensive biography unless the user requested one.
Stop after the question has been fully answered.

IMPORTANT: If you don't have enough information to answer the question, or find a contradiction, stop and state the problem.

Avoid markdown.  Format for a text terminal window.
`

const finalPrompt = `Answer the user's question using the supplied family-tree data.
State the main conclusion first, followed by the supporting genealogical facts.
Stop after the question has been fully answered.
Avoid markdown, format for a terminal window.
If you have marriage or divorce dates for couples, you may refer to them as spouses, husband, wife.
When no marriage date exists, you must only use the term partner.
When the user asks generically about a person, provide their name and dates and places of birth and death.
IMPORTANT: If you don't have enough information to answer the question, or find a contradiction, stop and state the problem.
`

func main() {
	var showData bool
	var onlyFinal bool

	model := "google/gemma-4-31B-it"
	url := "https://router.huggingface.co/v1/chat/completions"


	flag.StringVar(&model, "model", model, "LLM model to use")
	flag.StringVar(&url, "url", url, "inference engine to use")

	flag.BoolVar(&showData, "data", false, "show raw data sent to the LLM")
	flag.BoolVar(&onlyFinal, "final", false, "show only final results")
	flag.Parse()

	question := flag.Args()[0]

	fmt.Printf("PROMPT: %s\n", question)
	d := NewData("mrh-tree.ged")

	//url := "http://100.64.0.9:11434/v1/chat/completions"
	//url := "http://100.64.0.128:8000/v1/chat/completions"
	//url := "https://router.huggingface.co/v1/chat/completions"
	//model := "google/gemma-4-26B-A4B-it"
	//model := "openai/gpt-oss-120b"

	llm := NewLLM(model, url)

	llm.systemPrompt = functionPrompt
	llm.temp = 0.0

	ids := make(map[int]bool)
	gids := make(map[int]bool)

	//ids[0] = true
	hint := ""
	facts := make([]string, 0, 10)
	final := false
	for round := 0; round < 100; round++ {
		
		prompt := fmt.Sprintf("My ID is: %d.  %s\n", 0, question)
		data := ""

		if hint != "" {
			r := make(map[string]string)
			r["HINT"] = hint
			j, _ := json.Marshal(r)
			data += string(j) + "\n"
		}
		if len(facts) > 0 {
			r := make(map[string][]string)
			r["remembered facts"] = facts
			j, _ := json.Marshal(r)
			data += string(j) + "\n"			
		}
		
		for id := range(ids) {
			j := d.Info(id)
			data += j
		}
		for id := range(gids) {
			j := d.GEDCOMInfo(id)
			data += j
		}

		if showData {
			fmt.Println(data)
		}

		if final {
			llm.systemPrompt = finalPrompt
			llm.temp = 0.1
			
			fmt.Println("---- no further progress made, getting final answer --------------")
		}
		resp := ""
		for i := 0; resp == ""; i++ {
			var err error
			resp, err = llm.Chat(prompt, data)
			if resp == "" {
				fmt.Printf("--- no answer from LLM, tryig again: %s\n", err)
				if i > 2 {
					os.Exit(1)
				}
			}
		}


		fmt.Printf("------------------ %s worked on %d bytes of data for %s ------------\n", llm.model, len(data), llm.elapsed)
		if !onlyFinal || final {
			fmt.Println(resp)
			if final {break}
			fmt.Println("------------------")
		}

		// forget what we last remembered
		hint = ""
		// count current ids, see if we make progress later
		num_ids := len(ids) + len(gids)

		// look for function calls with string arguments
		re := regexp.MustCompile(`(?m)^([A-Z]+) (.+)$`)

		matches := re.FindAllStringSubmatch(resp, -1)
		for _, m := range matches {
			//fmt.Printf("m1: '%s' m2: '%s'\n", m[1], m[2])
			v := m[1]
			arg := m[2]
			if v == "SEARCH" || v == "BIRTH" || v == "DEATH" {
				searched := d.Search(v, arg)
				for id := range searched {
					ids[id] = true
				}
			}
			if v == "HINT" {
				hint = arg
			}
			if v == "REMEMBER" {
				fact := arg
				facts = append(facts, fact)
			}
			id, err := strconv.Atoi(arg)
			if err == nil {
				if v == "INFO" {
					ids[id] = true
				}
				if v == "GEDCOM" {
					gids[id] = true
				}
			}
		}


		num_ids2 := len(ids) + len(gids)

		// we have completed giving the LLM data
		if num_ids2 == num_ids {
			final = true
		} else {
			fmt.Printf("----- New records added by LLM round %d: %d (total: %d)\n", round, num_ids2 - num_ids, num_ids2)
		}
	}
}
