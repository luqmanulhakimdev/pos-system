# Entity relationship diagram

The diagram shows the planned core relationships. Exact columns and constraints will be defined by migrations as each domain is implemented.

```mermaid
erDiagram
  USERS ||--o{ USER_ROLES : has
  ROLES ||--o{ USER_ROLES : assigned
  ROLES ||--o{ ROLE_PERMISSIONS : grants
  PERMISSIONS ||--o{ ROLE_PERMISSIONS : included
  CATEGORIES ||--o{ PRODUCTS : groups
  PRODUCTS ||--|| INVENTORY : tracks
  PRODUCTS ||--o{ STOCK_MOVEMENTS : records
  CUSTOMERS ||--o{ ORDERS : places
  USERS ||--o{ ORDERS : creates
  ORDERS ||--|{ ORDER_ITEMS : contains
  PRODUCTS ||--o{ ORDER_ITEMS : snapshots
  ORDERS ||--o{ PAYMENTS : settles
  USERS ||--o{ AUDIT_LOGS : performs
```
