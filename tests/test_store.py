import pytest

from wowc_advisor.store import DataStore, DuckDBStore


@pytest.fixture(scope="module")
def store(dataset):
    return DuckDBStore(dataset)


def test_implements_protocol(store):
    assert isinstance(store, DataStore)


@pytest.mark.parametrize(
    ("id", "name"),
    [(1, "Linen Cloth"), (3, "Sunfire Staff"), (99, None)],
)
def test_get(store, id, name):
    row = store.get("items", id)
    assert (row["name"] if row else None) == name


@pytest.mark.parametrize(
    ("columns", "ids"),
    [
        ({"quality": "common"}, [1, 2, 4]),
        ({"quality": "common", "level": 60}, []),
        ({"level": 60}, [3]),
        ({}, [1, 2, 3, 4]),
    ],
)
def test_filter(store, columns, ids):
    assert [r["id"] for r in store.filter("items", **columns)] == ids


@pytest.mark.parametrize(
    ("text", "limit", "ids"),
    [("linen", 20, [1, 4]), ("CLOTH", 20, [1, 2]), ("linen", 1, [1]), ("zzz", 20, [])],
)
def test_search_name(store, text, limit, ids):
    assert [r["id"] for r in store.search_name("items", text, limit)] == ids


@pytest.mark.parametrize(
    ("call", "error"),
    [
        (lambda s: s.get("nope", 1), KeyError),
        (lambda s: s.get("../items", 1), ValueError),
        (lambda s: s.filter("items", **{"id; DROP": 1}), ValueError),
    ],
)
def test_errors(store, call, error):
    with pytest.raises(error):
        call(store)
