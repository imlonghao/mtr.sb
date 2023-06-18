import React, {useEffect, useState} from "react";
import {Button, Checkbox, Col, Form, Input, message, Row, Select, Table} from "antd";
import {ColumnsType} from "antd/es/table";
import {useOutletContext, useSearchParams} from "react-router-dom";
import {ipGeo, serverMap, serverStruct} from "./Root";
import ReactCountryFlag from "react-country-flag";

const { Option } = Select;

interface TracerouteTable {
  key: React.Key;
  seq: number;
  ip: string;
  ip_geo: ipGeo;
  rtt: number;
}

const columns: ColumnsType<TracerouteTable> = [
  {
    title: 'Seq',
    dataIndex: 'seq',
  },
  {
    title: 'IP',
    dataIndex: 'ip',
    className: 'ant-table-cell-ellipsis',
  },
  {
    title: 'Location',
    dataIndex: 'ip_geo',
    className: 'ant-table-cell-ellipsis',
    render: (_, { ip_geo }) => {
      if (ip_geo.country !== undefined) {
        return <>
          {ip_geo.city}, {ip_geo.region}, {ip_geo.country} [
          <a href={`https://bgp.tools/as/${ip_geo.asn}`} target="_blank" rel="noreferrer" title={ip_geo.asn_name}>
            AS{ip_geo.asn}
          </a>]
        </>
      }
      return ""
    },
  },
  {
    title: 'rDNS',
    dataIndex: 'ip_geo',
    render: (_, { ip_geo }) => ip_geo.rdns,
  },
  {
    title: 'RTT',
    dataIndex: 'rtt',
  },
];

export default function Traceroute() {
  let [searchParams, setSearchParams] = useSearchParams();
  const [form] = Form.useForm();
  const [data, setData] = useState([] as TracerouteTable[]);
  const [node, setNode] = useState(searchParams.get("n"));
  const [target, setTarget] = useState(searchParams.get("t"));
  const [protocol, setProtocol] = useState(searchParams.get("p") === null ? "0" : searchParams.get("p"));
  const [rd, setRd] = useState(searchParams.get("rd") === null ? "1" : searchParams.get("rd"));
  const [start, setStart] = useState(false);
  const [getIP, serverList] = useOutletContext() as [(ip: string) => ipGeo, serverMap];
  const [resolvedIP, setResolvedIP] = useState("N/A");
  const [messageApi, contextHolder] = message.useMessage();

  useEffect(() => {
    if (!start || target === "") {
      return
    }
    setData([])
    const sse = new EventSource(`/api/traceroute?n=${node}&t=${target}&p=${protocol}&rd=${rd}`);
    sse.onmessage = (ev) => {
      let parsed = JSON.parse(ev.data)

      if (parsed.lookup !== undefined) {
        setResolvedIP(parsed.lookup.ip)
        return
      }

      if (parsed.reply !== undefined) {
        const t = {
          key: parsed.reply.seq,
          seq: parsed.reply.seq,
          ip : parsed.reply.ip,
          rtt : parsed.reply.rtt,
        } as TracerouteTable
        setData((prev) => {
          const newData = [...prev]
          newData.push(t)
          return newData
        })
        return
      }

      if (parsed.timeout !== undefined) {
        const t = {
          key: parsed.timeout.seq,
          seq: parsed.timeout.seq,
          ip : "*"
        } as TracerouteTable
        setData((prev) => {
          const newData = [...prev]
          newData.push(t)
          return newData
        })
        return
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
    const r: TracerouteTable[] = []
    data.forEach((x) => {
      x.ip_geo = getIP(x.ip)
      r.push(x)
    })
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

  const submit_event = () => {
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
  }

  return <>
    {contextHolder}
    <h1>Traceroute</h1>
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
            <Input placeholder="" onKeyUp={(event) => {
              if (event.key !== "Enter" && event.key !== "NumpadEnter") {
                return
              }
              submit_event()
            }}/>
          </Form.Item>
        </Col>
        <Col xs={12} sm={10} lg={5}>
          <Form.Item label="Protocol" name="Protocol" initialValue={protocol}>
            <Select>
              <Option value="0">Auto</Option>
              <Option value="1">IPv4</Option>
              <Option value="2">IPv6</Option>
            </Select>
          </Form.Item>
        </Col>
        <Col xs={12} sm={6} lg={3}>
          <Form.Item name="RD" valuePropName="checked">
            <Checkbox defaultChecked={rd === "1"}>Remote DNS</Checkbox>
          </Form.Item>
        </Col>
        <Col xs={12} sm={4} lg={2}>
          <Form.Item>
            <Button type="primary" onClick={() => {
              submit_event()
            }} disabled={start}>Start</Button>
          </Form.Item>
        </Col>
        <Col xs={12} sm={4} lg={2}>
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
