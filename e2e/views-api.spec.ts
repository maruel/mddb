// E2E tests for table views API: filters, sorts, and view CRUD.

import { test, expect, registerUser, getWorkspaceId, createClient } from './helpers';
import type { DataRecordResponse } from '../sdk/types.gen';

test.describe('Table Views API', () => {
  test('create view and list records with filter', async ({ page, request }) => {
    const { token } = await registerUser(request, 'views-api');
    const client = createClient(request, token);

    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });
    const wsID = await getWorkspaceId(page);

    // Create Table
    const tableData = await client.ws(wsID).nodes.table.createTable('0', {
      title: 'Views Test Table',
      properties: [
        { name: 'Name', type: 'text' },
        { name: 'Age', type: 'number' },
      ],
    });
    const tableID = tableData.id;

    await client.ws(wsID).nodes.table.records.createRecord(tableID, { data: { Name: 'Alice', Age: 25 } });
    await client.ws(wsID).nodes.table.records.createRecord(tableID, { data: { Name: 'Bob', Age: 10 } });

    // Verify all records returned by default
    const defaultListParams = { ViewID: '', Filters: '', Sorts: '', Offset: 0, Limit: 100 };
    const listData = await client.ws(wsID).nodes.table.records.listRecords(tableID, defaultListParams);
    expect(listData.records.length).toBe(2);

    // Create a View with filter (Age > 18)
    const viewData = await client.ws(wsID).nodes.views.createView(tableID, { name: 'Adults', type: 'table' });
    const viewID = viewData.id;
    await client.ws(wsID).nodes.views.updateView(tableID, viewID, {
      filters: [{ property: 'Age', operator: 'gt', value: 18 }],
    });

    // List with ViewID → only Alice
    const viewListParams = { ViewID: viewID, Filters: '', Sorts: '', Offset: 0, Limit: 100 };
    const listViewData = await client.ws(wsID).nodes.table.records.listRecords(tableID, viewListParams);
    expect(listViewData.records.length).toBe(1);
    expect((listViewData.records[0] as DataRecordResponse).data.Name).toBe('Alice');

    // Ad-hoc filter (Age < 15) → only Bob
    const filterListParams = {
      ViewID: '',
      Filters: JSON.stringify([{ property: 'Age', operator: 'lt', value: 15 }]),
      Sorts: '',
      Offset: 0,
      Limit: 100,
    };
    const listAdHocData = await client.ws(wsID).nodes.table.records.listRecords(tableID, filterListParams);
    expect(listAdHocData.records.length).toBe(1);
    expect((listAdHocData.records[0] as DataRecordResponse).data.Name).toBe('Bob');
  });

  test('sort records via API', async ({ page, request }) => {
    const { token } = await registerUser(request, 'views-sort-api');
    const client = createClient(request, token);

    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });
    const wsID = await getWorkspaceId(page);

    const tableData = await client.ws(wsID).nodes.table.createTable('0', {
      title: 'Sort API Table',
      properties: [
        { name: 'Name', type: 'text' },
        { name: 'Score', type: 'number' },
      ],
    });
    const tableID = tableData.id;

    await client.ws(wsID).nodes.table.records.createRecord(tableID, { data: { Name: 'Charlie', Score: 50 } });
    await client.ws(wsID).nodes.table.records.createRecord(tableID, { data: { Name: 'Alice', Score: 90 } });
    await client.ws(wsID).nodes.table.records.createRecord(tableID, { data: { Name: 'Bob', Score: 70 } });

    // Sort by Name asc
    const nameAscParams = {
      ViewID: '',
      Filters: '',
      Sorts: JSON.stringify([{ property: 'Name', direction: 'asc' }]),
      Offset: 0,
      Limit: 100,
    };
    const nameAsc = await client.ws(wsID).nodes.table.records.listRecords(tableID, nameAscParams);
    const namesAsc = nameAsc.records.map((r: DataRecordResponse) => r.data.Name);
    expect(namesAsc).toEqual(['Alice', 'Bob', 'Charlie']);

    // Sort by Score desc
    const scoreDescParams = {
      ViewID: '',
      Filters: '',
      Sorts: JSON.stringify([{ property: 'Score', direction: 'desc' }]),
      Offset: 0,
      Limit: 100,
    };
    const scoreDesc = await client.ws(wsID).nodes.table.records.listRecords(tableID, scoreDescParams);
    const scoresDesc = scoreDesc.records.map((r: DataRecordResponse) => r.data.Name);
    expect(scoresDesc).toEqual(['Alice', 'Bob', 'Charlie']);

    // Compound sort: Score asc, then Name asc (add ties)
    await client.ws(wsID).nodes.table.records.createRecord(tableID, { data: { Name: 'Diana', Score: 70 } });
    const compoundParams = {
      ViewID: '',
      Filters: '',
      Sorts: JSON.stringify([
        { property: 'Score', direction: 'asc' },
        { property: 'Name', direction: 'asc' },
      ]),
      Offset: 0,
      Limit: 100,
    };
    const compound = await client.ws(wsID).nodes.table.records.listRecords(tableID, compoundParams);
    const compoundNames = compound.records.map((r: DataRecordResponse) => r.data.Name);
    expect(compoundNames).toEqual(['Charlie', 'Bob', 'Diana', 'Alice']);
  });

  test('view persists sorts across requests', async ({ page, request }) => {
    const { token } = await registerUser(request, 'views-sort-persist');
    const client = createClient(request, token);

    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });
    const wsID = await getWorkspaceId(page);

    const tableData = await client.ws(wsID).nodes.table.createTable('0', {
      title: 'Sort Persist Table',
      properties: [{ name: 'Name', type: 'text' }],
    });
    const tableID = tableData.id;

    await client.ws(wsID).nodes.table.records.createRecord(tableID, { data: { Name: 'Zebra' } });
    await client.ws(wsID).nodes.table.records.createRecord(tableID, { data: { Name: 'Apple' } });

    // Create view with sort
    const view = await client.ws(wsID).nodes.views.createView(tableID, { name: 'Sorted', type: 'table' });
    await client.ws(wsID).nodes.views.updateView(tableID, view.id, {
      sorts: [{ property: 'Name', direction: 'asc' }],
    });

    // Query with ViewID (sort applied server-side)
    const params = { ViewID: view.id, Filters: '', Sorts: '', Offset: 0, Limit: 100 };
    const data = await client.ws(wsID).nodes.table.records.listRecords(tableID, params);
    const names = data.records.map((r: DataRecordResponse) => r.data.Name);
    expect(names).toEqual(['Apple', 'Zebra']);
  });

  test('filter and sort combined', async ({ page, request }) => {
    const { token } = await registerUser(request, 'views-filter-sort');
    const client = createClient(request, token);

    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });
    const wsID = await getWorkspaceId(page);

    const tableData = await client.ws(wsID).nodes.table.createTable('0', {
      title: 'Filter Sort Table',
      properties: [
        { name: 'Name', type: 'text' },
        { name: 'Age', type: 'number' },
      ],
    });
    const tableID = tableData.id;

    await client.ws(wsID).nodes.table.records.createRecord(tableID, { data: { Name: 'Charlie', Age: 30 } });
    await client.ws(wsID).nodes.table.records.createRecord(tableID, { data: { Name: 'Alice', Age: 25 } });
    await client.ws(wsID).nodes.table.records.createRecord(tableID, { data: { Name: 'Bob', Age: 10 } });
    await client.ws(wsID).nodes.table.records.createRecord(tableID, { data: { Name: 'Diana', Age: 22 } });

    // Filter: Age >= 20, Sort: Name desc
    const params = {
      ViewID: '',
      Filters: JSON.stringify([{ property: 'Age', operator: 'gte', value: 20 }]),
      Sorts: JSON.stringify([{ property: 'Name', direction: 'desc' }]),
      Offset: 0,
      Limit: 100,
    };
    const data = await client.ws(wsID).nodes.table.records.listRecords(tableID, params);
    const names = data.records.map((r: DataRecordResponse) => r.data.Name);
    expect(names).toEqual(['Diana', 'Charlie', 'Alice']);
  });
});
