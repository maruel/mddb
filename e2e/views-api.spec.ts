import { test, expect, registerUser, getWorkspaceId, createClient } from './helpers';
import type { DataRecordResponse } from '../sdk/types.gen';

test.describe('Table Views API', () => {
  test('create view and list records with filter', async ({ page, request }) => {
    const { token } = await registerUser(request, 'views-api');
    const client = createClient(request, token);
    
    // We need to visit the page to get the workspace ID, or we can just fetch it via API if we knew how to get user info.
    // The helper `getWorkspaceId` relies on page navigation.
    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });
    const wsID = await getWorkspaceId(page);

    // 1. Create Table
    const tableData = await client.ws(wsID).nodes.table.createTable('0', {
      title: 'Views Test Table',
      properties: [
        { name: 'Name', type: 'text' },
        { name: 'Age', type: 'number' },
      ],
    });
    const tableID = tableData.id;

    // 2. Add Records
    // Alice: 25
    await client.ws(wsID).nodes.table.records.createRecord(tableID, {
      data: { Name: 'Alice', Age: 25 },
    });
    // Bob: 10
    await client.ws(wsID).nodes.table.records.createRecord(tableID, {
      data: { Name: 'Bob', Age: 10 },
    });

    // 3. Verify all records returned by default
    const defaultListParams = { ViewID: '', Filters: '', Sorts: '', Offset: 0, Limit: 100 };
    const listData = await client.ws(wsID).nodes.table.records.listRecords(tableID, defaultListParams);
    expect(listData.records.length).toBe(2);

    // 4. Create a View (Filter Age > 18)
    const viewData = await client.ws(wsID).nodes.views.createView(tableID, {
      name: 'Adults',
      type: 'table',
    });
    const viewID = viewData.id;

    // Update the view with filters (could be done in create if API supported it, but our API splits it? 
    // Wait, CreateViewRequest only has Name and Type. We must update to add filters.
    await client.ws(wsID).nodes.views.updateView(tableID, viewID, {
      filters: [
        { property: 'Age', operator: 'gt', value: 18 }
      ]
    });

    // 5. List Records with ViewID
    const viewListParams = { ViewID: viewID, Filters: '', Sorts: '', Offset: 0, Limit: 100 };
    const listViewData = await client.ws(wsID).nodes.table.records.listRecords(tableID, viewListParams);
    expect(listViewData.records.length).toBe(1);
    expect((listViewData.records[0] as DataRecordResponse).data.Name).toBe('Alice');

    // 6. List Records with Ad-hoc Filter (Age < 15)
    const filterListParams = {
      ViewID: '',
      Filters: JSON.stringify([{ property: 'Age', operator: 'lt', value: 15 }]),
      Sorts: '',
      Offset: 0,
      Limit: 100
    };
    const listAdHocData = await client.ws(wsID).nodes.table.records.listRecords(tableID, filterListParams);
    expect(listAdHocData.records.length).toBe(1);
    expect((listAdHocData.records[0] as DataRecordResponse).data.Name).toBe('Bob');
  });
});
