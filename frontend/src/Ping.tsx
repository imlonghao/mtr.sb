import React, {ReactNode, useEffect, useState} from "react";
import {Button, Checkbox, Col, Form, Input, message, Row, Select, Table} from "antd";
import {ColumnsType} from "antd/es/table";
import {PingResponse} from "../../proto/mtrsb";
import {useOutletContext, useSearchParams} from "react-router-dom";
import {ipGeo, serverMap} from "./Root";
import ReactCountryFlag from "react-country-flag";
import {LinkOutlined} from "@ant-design/icons";

const { Option } = Select;

interface DataStruct {
  [key: string]: PingResponse[];
}

interface PingTable {
  key: React.Key;
  country: string;
  location: string;
  provider: string;
  aff: string;
  ip: string;
  ip_geo: ipGeo;
  loss: number;
  sent: number;
  last: number;
  avg: number;
  best: number;
  worst: number;
  stdev: number;
  rttList: number[];
  node: string;
}

const columns: ColumnsType<PingTable> = [
  {
    title: 'Location',
    dataIndex: 'location',
    width: 150,
    fixed: 'left',
    render: (_, { country, location }) => (
      <>
        <ReactCountryFlag title={country} countryCode={country} svg style={{
          width: '21px',
          height: '15px',
          marginRight: '5px',
        }} />
        {location}
      </>
    ),
  },
  {
    title: 'Provider',
    dataIndex: 'provider',
    width: 150,
    render: (_, { provider, aff }) => {
      if (aff !== "") {
        return (
          <>
            <a href={aff} target="_blank" rel="noreferrer">{provider} <LinkOutlined /></a>
          </>
        )
      } else {
        return (
          <>
            {provider}
          </>
        )
      }
    },
  },
  {
    title: 'IP',
    dataIndex: 'ip',
    className: 'ant-table-cell-ellipsis',
    width: 600,
    render: (_, { ip, ip_geo, node }) => {
      if (ip_geo.country === undefined) {
        return <>{ip}</>
      } else {
        return <>
          {ip} <small style={{color: "gray"}}>
          {ip_geo.city}, {ip_geo.region}, {ip_geo.country} [
          <a href={`https://bgp.tools/as/${ip_geo.asn}`} target="_blank" rel="noreferrer" title={ip_geo.asn_name}>
            AS{ip_geo.asn}
          </a>]
          [<a href={`/traceroute?n=${node}&t=${ip}`} target="_blank" rel="noreferrer">T</a>]
          [<a href={`/mtr?n=${node}&t=${ip}`} target="_blank" rel="noreferrer">M</a>]
        </small>
        </>
      }
    },
  },
  {
    title: 'Loss',
    dataIndex: 'loss',
    width: 100,
    sorter: (a, b) => a.loss - b.loss,
  },
  {
    title: 'Sent',
    dataIndex: 'sent',
    width: 100,
    sorter: (a, b) => a.sent - b.sent,
  },
  {
    title: 'Last',
    dataIndex: 'last',
    width: 100,
    sorter: (a, b) => a.last - b.last,
  },
  {
    title: 'Avg',
    dataIndex: 'avg',
    width: 100,
    sorter: (a, b) => a.avg - b.avg,
  },
  {
    title: 'Best',
    dataIndex: 'best',
    defaultSortOrder: 'ascend',
    width: 100,
    sorter: (a, b) => a.best - b.best,
  },
  {
    title: 'Worst',
    dataIndex: 'worst',
    width: 100,
    sorter: (a, b) => a.worst - b.worst,
  },
  {
    title: 'Stdev',
    dataIndex: 'stdev',
    width: 100,
    sorter: (a, b) => a.stdev - b.stdev,
  },
  {
    title: 'Graph',
    dataIndex: 'graph',
    width: 100,
    render: (_, { rttList }) => {
      const lines = [] as ReactNode[]
      const max = Math.max.apply(null, rttList)
      let idx = -0.5
      rttList.forEach((x) => {
        idx += 1
        if (x === -1) {
          lines.push(<line x1={idx} y1="10" x2={idx} y2="0" style={{stroke:"#f56666"}} />)
          return
        }
        lines.push(<line x1={idx} y1="10" x2={idx} y2={10-x/max*10} style={{stroke:"#48bb77"}} />)
      })
      return <>
        <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10" height="20">
          {lines}
        </svg>
      </>
    },
  },
];

function dev(arr: number[]) : number {
  const n = arr.length
  const mean = arr.reduce((a, b) => a + b, 0) / n
  return Math.sqrt(arr.map(x => Math.pow(x - mean, 2)).reduce((a, b) => a + b, 0) / n)
}

export default function Ping() {
  let [searchParams, setSearchParams] = useSearchParams();
  const [form] = Form.useForm();
  const [data, setData] = useState({} as DataStruct);
  const [target, setTarget] = useState(searchParams.get("t"));
  const [protocol, setProtocol] = useState(searchParams.get("p") === null ? "0" : searchParams.get("p"));
  const [rd, setRd] = useState(searchParams.get("rd") === null ? "1" : searchParams.get("rd"));
  const [start, setStart] = useState(false);
  const [getIP, serverList] = useOutletContext() as [(ip: string) => ipGeo, serverMap];
  const [messageApi] = message.useMessage();

  useEffect(() => {
    if (!start || target === "") {
      return
    }
    setData({})
    const sse = new EventSource(`/api/ping?t=${target}&p=${protocol}&rd=${rd}`);
    sse.onmessage = (ev) => {
      let parsed = JSON.parse(ev.data)
      let nodeName = parsed.node
      setData((prev) => {
        const newData = {...prev}
        if (newData[nodeName] === undefined) {
          newData[nodeName] = [parsed.data]
        } else {
          newData[nodeName] = [...prev[nodeName], parsed.data]
        }
        return newData
      })
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
  }, [start, target, protocol, rd, messageApi]);

  let tableData = () => {
    let r: PingTable[] = []
    for (const [key, value] of Object.entries<PingResponse[]>(data)) {
      if (value.length === 1) {
        continue
      }
      const rttList = value.filter((x) => x.reply !== undefined).map((x)=>x.reply!.rtt!)
      const ip = value.filter((x) => x.lookup !== undefined).at(0)?.lookup!.ip!
      const graph = [] as number[]
      value.forEach((x) => {
        if (x.reply !== undefined) {
          graph.push(x.reply.rtt)
          return
        }
        if (x.timeout !== undefined) {
          graph.push(-1)
          return;
        }
      })
      r.push({
        key: key+value.length,
        country: serverList[key].country,
        location: serverList[key].location,
        provider: serverList[key].provider,
        aff: serverList[key].aff_link,
        ip: ip,
        ip_geo: getIP(ip),
        loss: value.filter((x) => x.timeout !== undefined).length,
        sent: value.filter((x) => x.reply !== undefined || x.timeout !== undefined).length,
        last: rttList.at(-1) ?? -1,
        avg: parseFloat((rttList.reduce((a, b) => a + b, 0) / rttList.length).toFixed(2)),
        best: Math.min.apply(null, rttList),
        worst: Math.max.apply(null, rttList),
        stdev: parseFloat(dev(rttList).toFixed(2)),
        rttList: graph,
        node: key,
      })
    }
    return r
  }

  return <>
    <h1>Ping</h1>
    <Form form={form}>
      <Row gutter={16}>
        <Col xs={24} sm={16} lg={6}>
          <Form.Item label="Target" name="Target" initialValue={target}>
            <Input placeholder="" />
          </Form.Item>
        </Col>
        <Col xs={24} sm={8} lg={6}>
          <Form.Item label="Protocol" name="Protocol" initialValue={protocol}>
            <Select>
              <Option value="0">Auto</Option>
              <Option value="1">IPv4</Option>
              <Option value="2">IPv6</Option>
            </Select>
          </Form.Item>
        </Col>
        <Col xs={24} sm={8} lg={6}>
          <Form.Item name="RD" valuePropName="checked">
            <Checkbox defaultChecked={rd === "1"}>Remote DNS</Checkbox>
          </Form.Item>
        </Col>
        <Col xs={12} sm={8} lg={3}>
          <Form.Item>
            <Button type="primary" onClick={() => {
              const _t = form.getFieldValue("Target").trim()
              const _p = form.getFieldValue("Protocol")
              const _rd = form.getFieldInstance("RD").input.checked ? "1" : "0"
              setTarget(_t)
              setProtocol(_p)
              setRd(_rd)
              setStart(true)
              setSearchParams({
                t: _t,
                p: _p,
                rd: _rd,
              });
            }} disabled={start}>Start</Button>
          </Form.Item>
        </Col>
        <Col xs={12} sm={8} lg={3}>
          <Form.Item>
            <Button danger type="primary" onClick={() => setStart(false)} disabled={!start}>Stop</Button>
          </Form.Item>
        </Col>
      </Row>
    </Form>
    <Table columns={columns} dataSource={tableData()} scroll={{ x: 'max-content' }} sticky size="small" pagination={false} />
  </>
}
