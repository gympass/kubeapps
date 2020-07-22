import { mount } from "enzyme";
import * as React from "react";
import { Provider } from "react-redux";
import { BrowserRouter as Router } from "react-router-dom";
import configureMockStore, { MockStore } from "redux-mock-store";
import thunk from "redux-thunk";

import { IStoreState } from "../../shared/types";

export const mockStore = configureMockStore([thunk]);

export const initialState = {
  apps: {},
  auth: {},
  catalog: {},
  charts: {},
  config: {},
  kube: {
    items: {},
  },
  clusters: {
    currentCluster: "default-cluster",
  },
  repos: {},
  operators: {},
} as IStoreState;

export const defaultStore = mockStore(initialState);

export const mountWrapper = (store: MockStore, children: any) =>
  mount(
    <Provider store={store}>
      <Router>{children}</Router>
    </Provider>,
  );
