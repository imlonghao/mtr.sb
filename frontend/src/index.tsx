import React from 'react';
import ReactDOM from 'react-dom/client';
import reportWebVitals from './reportWebVitals';
import {createBrowserRouter, RouterProvider} from "react-router-dom";
import Root from "./Root";
import Ping from "./Ping";
import Home from "./Home";
import Version from "./Version";
import Traceroute from "./Traceroute";
import Mtr from "./Mtr";
import Whois from "./Whois";

const router = createBrowserRouter([
  {
    path: "/",
    element: <Root />,
    children: [
      {
        index: true,
        element: <Home />
      },
      {
        path: "ping",
        element: <Ping />,
      },
      {
        path: "traceroute",
        element: <Traceroute />,
      },
      {
        path: "mtr",
        element: <Mtr />,
      },
      {
        path: "whois",
        element: <Whois />,
      },
      {
        path: "version",
        element: <Version />,
      },
    ],
  },
]);
const root = ReactDOM.createRoot(
  document.getElementById('root') as HTMLElement
);
root.render(
    <React.StrictMode>
      <RouterProvider router={router} />
    </React.StrictMode>
);

// If you want to start measuring performance in your app, pass a function
// to log results (for example: reportWebVitals(console.log))
// or send to an analytics endpoint. Learn more: https://bit.ly/CRA-vitals
reportWebVitals();
