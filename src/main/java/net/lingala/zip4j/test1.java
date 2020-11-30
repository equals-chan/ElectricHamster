package net.lingala.zip4j;

import jxl.write.WriteException;

import java.io.IOException;
import java.sql.SQLException;

public class test1 {
    public static void main(String[] args) throws IOException, WriteException, SQLException {
        sqliteConnecter sqliteConnecter = new sqliteConnecter();
        System.out.println(sqliteConnecter.isZipped("dskjfds"));


    }
}
