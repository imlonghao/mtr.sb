import React, {useEffect, useState} from "react";
import {Button, Checkbox, Col, Form, Input, message, Row, Select, Table} from "antd";
import {ColumnsType} from "antd/es/table";
import {useOutletContext, useSearchParams} from "react-router-dom";
import {ipGeo, serverMap, serverStruct} from "./Root";
import ReactCountryFlag from "react-country-flag";
import _ from "lodash";

const { Option } = Select;

interface MtrTable {
  key: React.Key;
  seq: string;
  ip: string;
  ip_geo: ipGeo;
  loss: number | string;
  sent: number | string;
  last: number | string;
  avg: number | string;
  best: number | string;
  worst: number | string;
  stdev: number | string;
  rttList: number[];
}

interface MtrCache {
  [key: string]: MtrItem
}

interface MtrItem {
  host: string;
  counter: number;
  rtt: number[];
}

const columns: ColumnsType<MtrTable> = [
  {
    title: '#',
    dataIndex: 'seq',
    width: 25,
  },
  {
    title: 'IP',
    dataIndex: 'ip',
    className: 'ant-table-cell-ellipsis',
    width: 600,
    render: (_, { ip, ip_geo }) => {
      if (ip_geo.country === undefined) {
        return <>{ip}</>
      } else {
        return <>
          {ip} <small style={{color: "gray"}}>
          {ip_geo.rdns} <br />
          <ReactCountryFlag title={ip_geo.country_short} countryCode={ip_geo.country_short} svg style={{
            width: '21px',
            height: '15px',
            marginRight: '5px',
          }} />
          {ip_geo.city}, {ip_geo.region}, {ip_geo.country} [
          <a href={`https://bgp.tools/as/${ip_geo.asn}`} target="_blank" rel="noreferrer">
            AS{ip_geo.asn} {ip_geo.asn_name}
          </a>]
        </small>
        </>
      }
    }
  },
  {
    title: 'Loss',
    dataIndex: 'loss',
    width: 100,
  },
  {
    title: 'Sent',
    dataIndex: 'sent',
    width: 100,
  },
  {
    title: 'Last',
    dataIndex: 'last',
    width: 100,
  },
  {
    title: 'Avg',
    dataIndex: 'avg',
    width: 100,
  },
  {
    title: 'Best',
    dataIndex: 'best',
    width: 100,
  },
  {
    title: 'Worst',
    dataIndex: 'worst',
    width: 100,
  },
  {
    title: 'Stdev',
    dataIndex: 'stdev',
    width: 100,
  },
];

function dev(arr: number[]) : number {
  const n = arr.length
  const mean = arr.reduce((a, b) => a + b, 0) / n
  return Math.sqrt(arr.map(x => Math.pow(x - mean, 2)).reduce((a, b) => a + b, 0) / n)
}

export default function Mtr() {
  let [searchParams, setSearchParams] = useSearchParams();
  const [form] = Form.useForm();
  const [data, setData] = useState({} as MtrCache);
  const [node, setNode] = useState(searchParams.get("n"));
  const [target, setTarget] = useState(searchParams.get("t"));
  const [protocol, setProtocol] = useState(searchParams.get("p") === null ? "0" : searchParams.get("p"));
  const [rd, setRd] = useState(searchParams.get("rd") === null ? "1" : searchParams.get("rd"));
  const [start, setStart] = useState(false);
  const [getIP, serverList] = useOutletContext() as [(ip: string) => ipGeo, serverMap];
  const [resolvedIP, setResolvedIP] = useState("N/A");
  const [messageApi] = message.useMessage();

  useEffect(() => {
    if (!start || target === "") {
      return
    }
    setData({})
    const sse = new EventSource(`/api/mtr?n=${node}&t=${target}&p=${protocol}&rd=${rd}`);
    sse.onmessage = (ev) => {
      let parsed = JSON.parse(ev.data)

      if (parsed.response.lookup !== undefined) {
        setResolvedIP(parsed.response.lookup.ip)
        return
      }

      if (parsed.pos === undefined) {
        parsed.pos = 0
      }

      if (parsed.response.transmit !== undefined) {
        setData((prev) => {
          const newData = _.cloneDeep(prev)
          if (prev[parsed.pos] === undefined) {
            newData[parsed.pos] = {
              host: "*",
              counter: 1,
              rtt: [],
            }
          } else {
            newData[parsed.pos].counter++
          }
          return newData
        })
      }

      if (parsed.response.host !== undefined) {
        setData((prev) => {
          const newData = _.cloneDeep(prev)
          if (prev[parsed.pos] === undefined) {
            newData[parsed.pos] = {
              host: parsed.response.host.ip,
              counter: 0,
              rtt: [],
            }
          } else {
            newData[parsed.pos].host = parsed.response.host.ip
          }
          return newData
        })
      }

      if (parsed.response.ping !== undefined) {
        setData((prev) => {
          const newData = _.cloneDeep(prev)
          if (prev[parsed.pos] === undefined) {
            newData[parsed.pos] = {
              host: "*",
              counter: 1,
              rtt: [parsed.response.ping.rtt],
            }
          } else {
            newData[parsed.pos].rtt.push(parsed.response.ping.rtt)
          }
          return newData
        })
      }
    };
    sse.onerror = () => {
      messageApi.open({
        type: 'error',
        content: 'Failed to connect to server, maybe being rate limited',
      });
      setStart(false)
    }
    return () => {
      sse.close()
      setStart(false)
    }
  }, [node, start, target, protocol, rd, messageApi]);

  const tableData = () => {
    const r: MtrTable[] = []
    for (const [key, value] of Object.entries<MtrItem>(data)) {
      const t = {
        key: key,
        seq: key,
        ip: value.host,
        ip_geo: getIP(value.host),
        loss: value.counter-value.rtt.length,
        sent: value.counter,
        last: value.rtt[value.rtt.length-1],
        avg: parseFloat((value.rtt.reduce((a, b) => a + b, 0) / value.rtt.length).toFixed(2)),
        best: Math.min.apply(null, value.rtt),
        worst: Math.max.apply(null, value.rtt),
        stdev: parseFloat(dev(value.rtt).toFixed(2))
      } as MtrTable
      if (t.ip === "*") {
        t.loss = "*"
        t.sent = "*"
        t.last = "*"
        t.avg = "*"
        t.best = "*"
        t.worst = "*"
        t.stdev = "*"
      }
      r.push(t)
      if (value.host === resolvedIP) {
        break
      }
    }
    return r
  }

  const nodeSelector = () => {
    const r = []
    for (const [key, value] of Object.entries<serverStruct>(serverList)) {
      r.push(<Option key={key} value={key}><ReactCountryFlag countryCode={value.country} svg style={{
        width: '21px',
        height: '15px',
        marginRight: '5px',
      }}/>{value.location} ({value.provider})</Option>)
    }
    return r
  }

  return <>
    <h1>Mtr</h1>
    <Form form={form}>
      <Row gutter={16}>
        <Col xs={24} sm={12} lg={6}>
          <Form.Item label="Node" name="Node" initialValue={node}>
            <Select>
              {nodeSelector()}
            </Select>
          </Form.Item>
        </Col>
        <Col xs={24} sm={12} lg={6}>
          <Form.Item label="Target" name="Target" initialValue={target}>
            <Input placeholder="" />
          </Form.Item>
        </Col>
        <Col xs={6} sm={10} lg={5}>
          <Form.Item label="Protocol" name="Protocol" initialValue={protocol}>
            <Select>
              <Option value="0">Auto</Option>
              <Option value="1">IPv4</Option>
              <Option value="2">IPv6</Option>
            </Select>
          </Form.Item>
        </Col>
        <Col xs={6} sm={6} lg={3}>
          <Form.Item name="RD" valuePropName="checked">
            <Checkbox defaultChecked={rd === "1"}>Remote DNS</Checkbox>
          </Form.Item>
        </Col>
        <Col xs={6} sm={4} lg={2}>
          <Form.Item>
            <Button type="primary" onClick={() => {
              setResolvedIP("N/A")

              const _n = form.getFieldValue("Node")
              const _t = form.getFieldValue("Target").trim()
              const _p = form.getFieldValue("Protocol")
              const _rd = form.getFieldInstance("RD").input.checked ? "1" : "0"
              setNode(_n)
              setTarget(_t)
              setProtocol(_p)
              setRd(_rd)
              setStart(true)
              setSearchParams({
                n: _n,
                t: _t,
                p: _p,
                rd: _rd,
              });
            }} disabled={start}>Start</Button>
          </Form.Item>
        </Col>
        <Col xs={6} sm={4} lg={2}>
          <Form.Item>
            <Button danger type="primary" onClick={() => setStart(false)} disabled={!start}>Stop</Button>
          </Form.Item>
        </Col>
      </Row>
    </Form>
    <span>Resolved IP: {resolvedIP}</span>
    <Table columns={columns} dataSource={tableData()} scroll={{ x: 'max-content' }} sticky size="small" pagination={false} />
  </>
}
