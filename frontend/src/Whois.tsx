import React, {useEffect, useState} from "react";
import {Button, Col, Form, Input, message, Row, Select} from "antd";
import {useSearchParams} from "react-router-dom";
import {Turnstile, TurnstileInstance} from "@marsidev/react-turnstile";

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
  const ref : React.MutableRefObject<TurnstileInstance|undefined> = React.useRef()

  useEffect(() => {
    if (!start || target === "" || token === "") {
      return
    }
    setData("")
    fetch(`/api/whois?t=${target}&s=${server}&token=${token}`).then(req => req.json()).then(data => {
      ref.current?.reset();
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
      ref.current?.reset();
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
    <Turnstile ref={ref} siteKey='0x4AAAAAAAGeiq0TQZ_Hozlv' onSuccess={setToken} style={{display: "none"}}/>
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
            <Select options={[
              {
                label: 'Default',
                options: [
                  { label: 'Auto', value: '' },
                  { label: 'BGP.Tools', value: 'bgp.tools' },
                ],
              },
              {
                label: 'Regional Internet Registry',
                options: [
                  { label: 'AFRINIC', value: 'whois.afrinic.net' },
                  { label: 'ARIN', value: 'whois.arin.net' },
                  { label: 'APNIC', value: 'whois.apnic.net' },
                  { label: 'LACNIC', value: 'whois.lacnic.net' },
                  { label: 'RIPE', value: 'whois.ripe.net' },
                ],
              },
              {
                label: 'Third-Part Internet Routing Registry',
                options: [
                  { label: 'RADB', value: 'whois.radb.net' },
                  { label: 'NTT', value: 'rr.ntt.net' },
                  { label: 'AltDB', value: 'whois.altdb.net' },
                ],
              },
            ]}>
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
    <pre style={{whiteSpace: "pre-wrap"}}>
      {data}
    </pre>
  </>
}
