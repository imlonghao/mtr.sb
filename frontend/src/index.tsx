import React from 'react';
import ReactDOM from 'react-dom/client';
import { createBrowserRouter, RouterProvider } from "react-router-dom";
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
