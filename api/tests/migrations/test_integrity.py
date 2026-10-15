"""Проверки ограничений целостности схемы.

Каждый тест проверяет, что БД отклоняет некорректные данные
или, наоборот, пропускает корректные. Ошибки ожидаются через
sqlalchemy.exc.IntegrityError.
"""

import pytest
from sqlalchemy import text
from sqlalchemy.exc import IntegrityError


def _insert_dag(session, dag_id: str = "test_dag", owner: str = "test_owner") -> int:
    row = session.execute(
        text("INSERT INTO dag (dag_id, owner) VALUES (:dag_id, :owner) RETURNING id"),
        {"dag_id": dag_id, "owner": owner},
    ).scalar_one()
    session.commit()
    return row


def _insert_dag_version(session, dag_id: int, version: int = 1) -> int:
    row = session.execute(
        text(
            """
            INSERT INTO dag_version (dag_id, version, schedule, start_date, source_yaml)
            VALUES (:dag_id, :version, '0 3 * * *', now(), 'yaml: stub')
            RETURNING id
            """
        ),
        {"dag_id": dag_id, "version": version},
    ).scalar_one()
    session.commit()
    return row


def _insert_task(session, dag_version_id: int, task_id: str = "t1") -> int:
    row = session.execute(
        text(
            """
            INSERT INTO task (dag_version_id, task_id, operator)
            VALUES (:dag_version_id, :task_id, 'bash')
            RETURNING id
            """
        ),
        {"dag_version_id": dag_version_id, "task_id": task_id},
    ).scalar_one()
    session.commit()
    return row


def _insert_dag_run(
    session,
    dag_id: int,
    dag_version_id: int,
    run_id: str,
    trigger_type: str = "manual",
    execution_date: str = "2026-10-01T00:00:00+00:00",
) -> int:
    row = session.execute(
        text(
            """
            INSERT INTO dag_run (run_id, dag_id, dag_version_id, execution_date, trigger_type)
            VALUES (:run_id, :dag_id, :dag_version_id, :execution_date, :trigger_type)
            RETURNING id
            """
        ),
        {
            "run_id": run_id,
            "dag_id": dag_id,
            "dag_version_id": dag_version_id,
            "execution_date": execution_date,
            "trigger_type": trigger_type,
        },
    ).scalar_one()
    session.commit()
    return row


def _insert_task_instance(session, run_id: int, task_id: int, state: str = "pending", max_tries: int = 3) -> int:
    row = session.execute(
        text(
            """
            INSERT INTO task_instance (run_id, task_id, state, max_tries)
            VALUES (:run_id, :task_id, :state, :max_tries)
            RETURNING id
            """
        ),
        {"run_id": run_id, "task_id": task_id, "state": state, "max_tries": max_tries},
    ).scalar_one()
    session.commit()
    return row


# 1. FK: task_instance с несуществующим run_id → ошибка
def test_fk_task_instance_run_id(session):
    dag_id = _insert_dag(session)
    dag_version_id = _insert_dag_version(session, dag_id)
    task_id = _insert_task(session, dag_version_id)

    with pytest.raises(IntegrityError):
        session.execute(
            text(
                """
                INSERT INTO task_instance (run_id, task_id, state, max_tries)
                VALUES (99999, :task_id, 'pending', 3)
                """
            ),
            {"task_id": task_id},
        )
        session.commit()
    session.rollback()


# 2. CHECK: task_instance с state='lalala' → ошибка
def test_check_task_instance_state(session):
    dag_id = _insert_dag(session)
    dag_version_id = _insert_dag_version(session, dag_id)
    task_id = _insert_task(session, dag_version_id)
    run_id = _insert_dag_run(session, dag_id, dag_version_id, "run_test_1")

    with pytest.raises(IntegrityError):
        session.execute(
            text(
                """
                INSERT INTO task_instance (run_id, task_id, state, max_tries)
                VALUES (:run_id, :task_id, 'lalala', 3)
                """
            ),
            {"run_id": run_id, "task_id": task_id},
        )
        session.commit()
    session.rollback()


# 3. UNIQUE: повторный dag_version (dag_id, version) → ошибка
def test_unique_dag_version(session):
    dag_id = _insert_dag(session)
    _insert_dag_version(session, dag_id, version=1)

    with pytest.raises(IntegrityError):
        session.execute(
            text(
                """
                INSERT INTO dag_version (dag_id, version, schedule, start_date, source_yaml)
                VALUES (:dag_id, 1, '0 3 * * *', now(), 'yaml: stub')
                """
            ),
            {"dag_id": dag_id},
        )
        session.commit()
    session.rollback()


# 4. UNIQUE: повторный task_instance (run_id, task_id) → ошибка
def test_unique_task_instance(session):
    dag_id = _insert_dag(session)
    dag_version_id = _insert_dag_version(session, dag_id)
    task_id = _insert_task(session, dag_version_id)
    run_id = _insert_dag_run(session, dag_id, dag_version_id, "run_test_1")

    _insert_task_instance(session, run_id, task_id)

    with pytest.raises(IntegrityError):
        session.execute(
            text(
                """
                INSERT INTO task_instance (run_id, task_id, state, max_tries)
                VALUES (:run_id, :task_id, 'pending', 3)
                """
            ),
            {"run_id": run_id, "task_id": task_id},
        )
        session.commit()
    session.rollback()


# 5. Partial UNIQUE: schedule дублируется → ошибка; manual дублируется → ок
def test_partial_unique_dag_run_schedule(session):
    dag_id = _insert_dag(session)
    dag_version_id = _insert_dag_version(session, dag_id)
    execution_date = "2026-10-01T00:00:00+00:00"

    _insert_dag_run(
        session, dag_id, dag_version_id, "run_sched_1",
        trigger_type="schedule", execution_date=execution_date,
    )

    # второй schedule на ту же дату — ошибка
    with pytest.raises(IntegrityError):
        session.execute(
            text(
                """
                INSERT INTO dag_run (run_id, dag_id, dag_version_id, execution_date, trigger_type)
                VALUES ('run_sched_2', :dag_id, :dag_version_id, :execution_date, 'schedule')
                """
            ),
            {"dag_id": dag_id, "dag_version_id": dag_version_id, "execution_date": execution_date},
        )
        session.commit()
    session.rollback()

    # manual на ту же дату — проходит
    _insert_dag_run(
        session, dag_id, dag_version_id, "run_manual_1",
        trigger_type="manual", execution_date=execution_date,
    )


# 6. CASCADE: удаление dag_run уносит task_instance
def test_cascade_dag_run_deletes_task_instance(session):
    dag_id = _insert_dag(session)
    dag_version_id = _insert_dag_version(session, dag_id)
    task_id = _insert_task(session, dag_version_id)
    run_id = _insert_dag_run(session, dag_id, dag_version_id, "run_test_1")
    _insert_task_instance(session, run_id, task_id)

    count_before = session.execute(
        text("SELECT COUNT(*) FROM task_instance WHERE run_id = :run_id"),
        {"run_id": run_id},
    ).scalar_one()
    assert count_before == 1

    session.execute(text("DELETE FROM dag_run WHERE id = :run_id"), {"run_id": run_id})
    session.commit()

    count_after = session.execute(
        text("SELECT COUNT(*) FROM task_instance WHERE run_id = :run_id"),
        {"run_id": run_id},
    ).scalar_one()
    assert count_after == 0


# 7. CHECK: task_dependency с task_id = depends_on_task_id → ошибка
def test_check_task_dependency_no_self(session):
    dag_id = _insert_dag(session)
    dag_version_id = _insert_dag_version(session, dag_id)
    _insert_task(session, dag_version_id, task_id="t1")

    with pytest.raises(IntegrityError):
        session.execute(
            text(
                """
                INSERT INTO task_dependency (dag_version_id, task_id, depends_on_task_id)
                VALUES (:dag_version_id, 't1', 't1')
                """
            ),
            {"dag_version_id": dag_version_id},
        )
        session.commit()
    session.rollback()
