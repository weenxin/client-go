## JsonPath

## 用例场景

kubectl 支持jsonpath输出：

### 创建测试pod

创建一个pod: `kubectl create -f ./util/jsonpath/testdata/pod.yaml`

获取json数据:  `kubectl get pod -o json`

### jsonpath获取结果

获取全部数据： `kubectl get pod test-jsonpath -o=jsonpath="{@}"`

### 只获取指定字段

- `kubectl get pod test-jsonpath -o=jsonpath="{.kind}"`
- `kubectl get pod test-jsonpath -o=jsonpath="{.metadata.name}"`
- `kubectl get pod test-jsonpath -o=jsonpath="{['metadata.name']}"` : 将对象当作map使用

### 寻找所有子field

- `kubectl get pod test-jsonpath -o=jsonpath="names : [{..name}] "`

会获取所有字段字段为name的结果：

```
names : [test-jsonpath kube-api-access-w9kjk kube-root-ca.crt nginx kube-api-access-w9kjk nginx]
```

### 数组访问

- 获取整个数组： `kubectl get pod test-jsonpath -o=jsonpath="{.spec.tolerations}"`

```json
[
    {
        "effect":"NoExecute",
        "key":"node.kubernetes.io/not-ready",
        "operator":"Exists",
        "tolerationSeconds":300
    },
    {
        "effect":"NoExecute",
        "key":"node.kubernetes.io/unreachable",
        "operator":"Exists",
        "tolerationSeconds":300
    }
]
```

- 获取所有元素： `kubectl get pod test-jsonpath -o=jsonpath="{.spec.tolerations[*]}"`

```json
    {
        "effect":"NoExecute",
        "key":"node.kubernetes.io/not-ready",
        "operator":"Exists",
        "tolerationSeconds":300
    },
    {
        "effect":"NoExecute",
        "key":"node.kubernetes.io/unreachable",
        "operator":"Exists",
        "tolerationSeconds":300
    }
```


- 获取第0个元素 `kubectl get pod test-jsonpath -o=jsonpath="{.spec.tolerations[0]}"`

```json
{
    "effect":"NoExecute",
    "key":"node.kubernetes.io/not-ready",
    "operator":"Exists",
    "tolerationSeconds":300
}
```

获取第[0~2]元素: `kubectl get pod test-jsonpath -o=jsonpath="{.spec.tolerations[0:2]}"`

```json
{"effect":"NoExecute","key":"node.kubernetes.io/not-ready","operator":"Exists","tolerationSeconds":300} {"effect":"NoExecute","key":"node.kubernetes.io/unreachable","operator":"Exists","tolerationSeconds":300}
```


### 筛选

-  获取所有effect为NoExecute的污点 ： `kubectl get pod test-jsonpath -o=jsonpath='{.spec.tolerations[?(@.effect=="NoExecute")]}'`

### 迭代


- 迭代数组：  `kubectl get pod test-jsonpath -o=jsonpath="{range .spec.tolerations[*]} effect:{.effect} , key:{.key} |  {end}"`

**注意.spec.tolerations[*] 如果是 .spec.tolerations {@}将会是数组本身**

```
 effect:NoExecute , key:node.kubernetes.io/not-ready |   effect:NoExecute , key:node.kubernetes.io/unreachable |
```

### 引用执行字符串

- `kubectl get pod test-jsonpath -o=jsonpath="{range .spec.tolerations[*]}effect:{.effect}{'\t'}key:{.key}{'\n'}{end}"`

会输出：

```
effect:NoExecute        key:node.kubernetes.io/not-ready
effect:NoExecute        key:node.kubernetes.io/unreachable
```

## 代码使用

[code](/util/jsonpath/jsonpath_test.go)


## 实现细节

`hello {range .items[*]}{.metadata.name}{"\t"}{end}`

机会解析出如下所示的树形列表

```
Root
        NodeText: hello
        NodeList
                NodeIdentifier: range
                NodeField: items
                NodeArray: [{0 false false} {0 false false} {0 false false}]
        NodeList
                NodeField: metadata
                NodeField: name
        NodeList
                NodeText
        NodeList
                NodeIdentifier: end
```

### Parse 阶段

输入 `hello {range .items[*]}{.metadata.name}{"\t"}{end}`

基于`Root`对象开始`Parse`

#### Parse Text对象

```go
func (p *Parser) parseText(cur *ListNode) error {
	for {
		if strings.HasPrefix(p.input[p.pos:], leftDelim) { //找到了第一个'{'
			if p.pos > p.start {
				cur.append(newText(p.consumeText())) // 将父亲节点加入一个子节点，类型为Text，结果为推进过程中消耗的字符
			}
			return p.parseLeftDelim(cur)
		}
		if p.next() == eof { //没有找到一直推进
			break
		}
	}
	// Correctly reached EOF.
	if p.pos > p.start {
		cur.append(newText(p.consumeText()))
	}
	return nil
}
```



#### ParseLeftDelim

`{range .items[*]}{.metadata.name}{"\t"}{end}`

```go
// parseLeftDelim scans the left delimiter, which is known to be present.
func (p *Parser) parseLeftDelim(cur *ListNode) error { //cur为root节点
	p.pos += len(leftDelim)
	p.consumeText() //消耗掉`{`
	newNode := newList() //新建一个节点
	cur.append(newNode) //加入到root节点的子节点中
	cur = newNode
	return p.parseInsideAction(cur) //解析内部的action
}
```

#### parseInsideAction

`range .items[*]}{.metadata.name}{"\t"}{end}`

```go
func (p *Parser) parseInsideAction(cur *ListNode) error { //当前节点为已经之前新建的节点
	prefixMap := map[string]func(*ListNode) error{
		rightDelim: p.parseRightDelim,
		"[?(":      p.parseFilter,
		"..":       p.parseRecursive,
	}
	for prefix, parseFunc := range prefixMap {
		if strings.HasPrefix(p.input[p.pos:], prefix) {
			return parseFunc(cur)
		}
	}

	//此时是一个 range operation

	switch r := p.next(); {
	case r == eof || isEndOfLine(r): //不符合
		return fmt.Errorf("unclosed action")
	case r == ' ': //去除所有空格
		p.consumeText()
	case r == '@' || r == '$': //the current object, just pass it
		p.consumeText()
	case r == '[': //第三次将会遇到
		return p.parseArray(cur)
	case r == '"' || r == '\'':
		return p.parseQuote(cur, r)
	case r == '.': // 第二次解析时遇到了.
		return p.parseField(cur)
	case r == '+' || r == '-' || unicode.IsDigit(r):
		p.backup()
		return p.parseNumber(cur)
	case isAlphaNumeric(r): //是字符，开始解析
		p.backup() //将标识符退回
		return p.parseIdentifier(cur)
	default:
		return fmt.Errorf("unrecognized character in action: %#U", r)
	}
	return p.parseInsideAction(cur)
}
```



```go
`range .items[*]}{.metadata.name}{"\t"}{end}`

// parseIdentifier scans build-in keywords, like "range" "end"
func (p *Parser) parseIdentifier(cur *ListNode) error { //当前节点还是新建的1号节点
	var r rune
	for {
		r = p.next()
		if isTerminator(r) {
			p.backup()
			break
		}
	}
	value := p.consumeText() //拿到range

	if isBool(value) { //不是true也不是false
		v, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("can not parse bool '%s': %s", value, err.Error())
		}

		cur.append(newBool(v))
	} else {
		cur.append(newIdentifier(value)) //1号节点增加了一个节点，为NodeIdentifier ： range
	}

	return p.parseInsideAction(cur) //继续解析内部action
}
```


`items[*]}{.metadata.name}{"\t"}{end}`

```go
// parseField scans a field until a terminator
func (p *Parser) parseField(cur *ListNode) error {
	p.consumeText() //忽略.
	for p.advance() { //持续向前
	}
	value := p.consumeText() //拿到item
	if value == "*" {
		cur.append(newWildcard())
	} else {
		cur.append(newField(strings.Replace(value, "\\", "", -1))) //将1号节点加入： NodeField： items
	}
	return p.parseInsideAction(cur) //继续处理
}
```

`[*]}{.metadata.name}{"\t"}{end}`
```go
case r == '[': //第三次将会遇到
		return p.parseArray(cur)
```

```go
// parseArray scans array index selection
func (p *Parser) parseArray(cur *ListNode) error {
Loop:
	for {
		switch p.next() { //持续推进
		case eof, '\n':
			return fmt.Errorf("unterminated array")
		case ']':  //知道遇到第一个']'
			break Loop
		}
	}
	text := p.consumeText() //此时text为 '[*]'
	text = text[1 : len(text)-1] //去掉左右 []
	if text == "*" { //text为 :
		text = ":"
	}

	//union operator
	strs := strings.Split(text, ",") //是否为union
	if len(strs) > 1 {
		union := []*ListNode{}
		for _, str := range strs {
			parser, err := parseAction("union", fmt.Sprintf("[%s]", strings.Trim(str, " ")))
			if err != nil {
				return err
			}
			union = append(union, parser.Root)
		}
		cur.append(newUnion(union))
		return p.parseInsideAction(cur)
	}

	// dict key， 是否为字段访问操作
	value := dictKeyRex.FindStringSubmatch(text) //text内容是否为'字符串' ，如果是，匹配出内部结果：字符串
	if value != nil {
		parser, err := parseAction("arraydict", fmt.Sprintf(".%s", value[1]))
		if err != nil {
			return err
		}
		for _, node := range parser.Root.Nodes {
			cur.append(node)
		}
		return p.parseInsideAction(cur)
	}

	//slice operator
	value = sliceOperatorRex.FindStringSubmatch(text) //是否匹配slice中数据索引
	if value == nil {
		return fmt.Errorf("invalid array index %s", text)
	}
	value = value[1:] //第一个为字符串本身，1为第一个索引，2：为第二个，3为第三个
	params := [3]ParamsEntry{}
	for i := 0; i < 3; i++ {
		if value[i] != "" {
			if i > 0 {
				value[i] = value[i][1:] //去掉：
			}
			if i > 0 && value[i] == "" {
				params[i].Known = false
			} else {
				var err error
				params[i].Known = true
				params[i].Value, err = strconv.Atoi(value[i])
				if err != nil {
					return fmt.Errorf("array index %s is not a number", value[i])
				}
			}
		} else {
			if i == 1 {
				params[i].Known = true
				params[i].Value = params[0].Value + 1
				params[i].Derived = true
			} else {
				params[i].Known = false
				params[i].Value = 0
			}
		}
	}
	cur.append(newArray(params))
	return p.parseInsideAction(cur) // 继续处理
}
```


`}{.metadata.name}{"\t"}{end}`


```go
prefixMap := map[string]func(*ListNode) error{
		rightDelim: p.parseRightDelim,
		"[?(":      p.parseFilter,
		"..":       p.parseRecursive,
	}
	for prefix, parseFunc := range prefixMap {
		if strings.HasPrefix(p.input[p.pos:], prefix) {
			return parseFunc(cur)
		}
	}
```


```go
// parseRightDelim scans the right delimiter, which is known to be present.
func (p *Parser) parseRightDelim(cur *ListNode) error {
	p.pos += len(rightDelim)
	p.consumeText() //消耗掉`}`
	return p.parseText(p.Root) //重新开始对跟对象渲染
}
```

此时还有`{.metadata.name}{"\t"}{end}`


### 执行阶段


```go
func (j *JSONPath) FindResults(data interface{}) ([][]reflect.Value, error) {
	if j.parser == nil {
		return nil, fmt.Errorf("%s is an incomplete jsonpath template", j.name)
	}

	cur := []reflect.Value{reflect.ValueOf(data)} //构造统一处理参数
	nodes := j.parser.Root.Nodes //拿到所有根对象
	fullResult := [][]reflect.Value{}
	for i := 0; i < len(nodes); i++ {
		node := nodes[i]
		results, err := j.walk(cur, node) //挨个处理
		if err != nil {
			return nil, err
		}

		// encounter an end node, break the current block
		if j.endRange > 0 && j.endRange <= j.inRange {
			j.endRange--
			j.lastEndNode = &nodes[i]
			break
		}
		// encounter a range node, start a range loop
		if j.beginRange > 0 {
			j.beginRange--
			j.inRange++
			if len(results) > 0 {
				for _, value := range results {
					j.parser.Root.Nodes = nodes[i+1:]
					nextResults, err := j.FindResults(value.Interface())
					if err != nil {
						return nil, err
					}
					fullResult = append(fullResult, nextResults...)
				}
			} else {
				// If the range has no results, we still need to process the nodes within the range
				// so the position will advance to the end node
				j.parser.Root.Nodes = nodes[i+1:]
				_, err := j.FindResults(nil)
				if err != nil {
					return nil, err
				}
			}
			j.inRange--

			// Fast forward to resume processing after the most recent end node that was encountered
			for k := i + 1; k < len(nodes); k++ {
				if &nodes[k] == j.lastEndNode {
					i = k
					break
				}
			}
			continue
		}
		fullResult = append(fullResult, results)
	}
	return fullResult, nil
}
```

#### 渲染文字

```
Root
        NodeText: hello
        NodeList
                NodeIdentifier: range
                NodeField: items
                NodeArray: [{0 false false} {0 false false} {0 false false}]
        NodeList
                NodeField: metadata
                NodeField: name
        NodeList
                NodeText
        NodeList
                NodeIdentifier: end
```

```go
// walk visits tree rooted at the given node in DFS order
func (j *JSONPath) walk(value []reflect.Value, node Node) ([]reflect.Value, error) {
	switch node := node.(type) {
	case *ListNode:
		return j.evalList(value, node)
	case *TextNode:
		return []reflect.Value{reflect.ValueOf(node.Text)}, nil //返回一个hello
	case *FieldNode:
		return j.evalField(value, node)
	case *ArrayNode:
		return j.evalArray(value, node)
	case *FilterNode:
		return j.evalFilter(value, node)
	case *IntNode:
		return j.evalInt(value, node)
	case *BoolNode:
		return j.evalBool(value, node)
	case *FloatNode:
		return j.evalFloat(value, node)
	case *WildcardNode:
		return j.evalWildcard(value, node)
	case *RecursiveNode:
		return j.evalRecursive(value, node)
	case *UnionNode:
		return j.evalUnion(value, node)
	case *IdentifierNode:
		return j.evalIdentifier(value, node)
	default:
		return value, fmt.Errorf("unexpected Node %v", node)
	}
}
```

### 渲染Range

```
        NodeList
                NodeIdentifier: range
                NodeField: items
                NodeArray: [{0 false false} {0 false false} {0 false false}]
        NodeList
                NodeField: metadata
                NodeField: name
        NodeList
                NodeText
        NodeList
                NodeIdentifier: end
```

```go
        node := nodes[i] //跟对象
		results, err := j.walk(cur, node) //Walk完成后，获取获取到了所有Item，并且beginRange = 1
		if err != nil {
			return nil, err
		}

		// encounter an end node, break the current block
		if j.endRange > 0 && j.endRange <= j.inRange {
			j.endRange--
			j.lastEndNode = &nodes[i]
			break
		}
		// encounter a range node, start a range loop
		if j.beginRange > 0 { //beginRange 为整数的原因为支持多重循环
			j.beginRange--
			j.inRange++ //
			if len(results) > 0 {
				for _, value := range results { //对每个元素循环操作，达到了循环的作用
					j.parser.Root.Nodes = nodes[i+1:]
					nextResults, err := j.FindResults(value.Interface()) //将所有元素都开始继续执行，此时因为beginRange = 0 ；
					//endRange = 0 ；  inRange =1 ，所以后续的操作都不会涉及到循环操作。
					if err != nil {
						return nil, err
					}
					fullResult = append(fullResult, nextResults...)
				}
			} else {
				// If the range has no results, we still need to process the nodes within the range
				// so the position will advance to the end node
				j.parser.Root.Nodes = nodes[i+1:]
				_, err := j.FindResults(nil)
				if err != nil {
					return nil, err
				}
			}
			j.inRange-- //处理完成后接触循环

			// Fast forward to resume processing after the most recent end node that was encountered
			for k := i + 1; k < len(nodes); k++ {
				if &nodes[k] == j.lastEndNode {
					i = k
					break
				}
			}
			continue
		}
		fullResult = append(fullResult, results) //加入结果
```


#### 处理输出

#### 结束

```
        NodeList
                NodeIdentifier: end
```

```go
func (j *JSONPath) FindResults(data interface{}) ([][]reflect.Value, error) {
	if j.parser == nil {
		return nil, fmt.Errorf("%s is an incomplete jsonpath template", j.name)
	}

	cur := []reflect.Value{reflect.ValueOf(data)}
	nodes := j.parser.Root.Nodes
	fullResult := [][]reflect.Value{}
	for i := 0; i < len(nodes); i++ {
		node := nodes[i]
		results, err := j.walk(cur, node)
		if err != nil {
			return nil, err
		}

		// encounter an end node, break the current block
		if j.endRange > 0 && j.endRange <= j.inRange { //此时j.endRange = 1 ； j.inRange  = 1
			j.endRange--
			j.lastEndNode = &nodes[i]
			break
		}
		// encounter a range node, start a range loop
		if j.beginRange > 0 { // j.beginRange = 0
			j.beginRange--
			j.inRange++
			if len(results) > 0 {
				for _, value := range results {
					j.parser.Root.Nodes = nodes[i+1:]
					nextResults, err := j.FindResults(value.Interface())
					if err != nil {
						return nil, err
					}
					fullResult = append(fullResult, nextResults...)
				}
			} else {
				// If the range has no results, we still need to process the nodes within the range
				// so the position will advance to the end node
				j.parser.Root.Nodes = nodes[i+1:]
				_, err := j.FindResults(nil)
				if err != nil {
					return nil, err
				}
			}
			j.inRange--

			// Fast forward to resume processing after the most recent end node that was encountered
			for k := i + 1; k < len(nodes); k++ {
				if &nodes[k] == j.lastEndNode {
					i = k
					break
				}
			}
			continue
		}
		fullResult = append(fullResult, results) //result 为空
	}
	return fullResult, nil
}

```








