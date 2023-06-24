import React, {useEffect, useState} from "react";
import {Button, Col, Form, Input, message, Row, Select} from "antd";
import {useSearchParams} from "react-router-dom";
import {Turnstile} from "@marsidev/react-turnstile";

const { Option } = Select;

export default function Whois() {
  let [searchParams, setSearchParams] = useSearchParams();
  const [form] = Form.useForm();
  const [data, setData] = useState("");
  const [target, setTarget] = useState(searchParams.get("t"));
  const [server, setServer] = useState(searchParams.get("s") === null ? "" : searchParams.get("s"));
  const [start, setStart] = useState(false);
  const [messageApi, contextHolder] = message.useMessage();
  const [token, setToken] = React.useState("")

  useEffect(() => {
    if (!start || target === "" || token === "") {
      return
    }
    setData("")
    fetch(`/api/whois?t=${target}&s=${server}&token=${token}`).then(req => req.json()).then(data => {
      if (data.ok !== true) {
        messageApi.open({
          type: 'error',
          content: data.data,
        });
        setStart(false)
        return
      }
      setData(data.data)
      setStart(false)
    }).catch(err => {
      messageApi.open({
        type: 'error',
        content: 'error when requesting API',
      });
      setStart(false)
      return
    })
  }, [start, target, token, server, messageApi]);

  const submit_event = () => {
    const _t = form.getFieldValue("Target").trim()
    const _s = form.getFieldValue("Server")
    setTarget(_t)
    setServer(_s)
    setStart(true)
    setSearchParams({
      t: _t,
      s: _s,
    });
  }

  return <>
    {contextHolder}
    <Turnstile siteKey='0x4AAAAAAAGeiq0TQZ_Hozlv' onSuccess={setToken} style={{display: "none"}}/>
    <h1>Whois</h1>
    <Form form={form}>
      <Row gutter={16}>
        <Col xs={24} sm={12}>
          <Form.Item label="Target" name="Target" initialValue={target}>
            <Input placeholder="Domain / IPv4 / IPv6 / ASN" onKeyUp={(event) => {
              if (event.key !== "Enter" && event.key !== "NumpadEnter") {
                return
              }
              submit_event()
            }}/>
          </Form.Item>
        </Col>
        <Col xs={24} sm={8}>
          <Form.Item label="Server" name="Server" initialValue={server}>
            <Select>
              <Option value="">Auto</Option>
              <Option value="whois.radb.net">RADB</Option>
              <Option value="rr.ntt.net">NTT</Option>
            </Select>
          </Form.Item>
        </Col>
        <Col xs={24} sm={4}>
          <Form.Item>
            <Button type="primary" onClick={() => {
              submit_event()
            }} disabled={start}>Start</Button>
          </Form.Item>
        </Col>
      </Row>
    </Form>
    <hr/>
    <code>
      {data}
    </code>
  </>
}
