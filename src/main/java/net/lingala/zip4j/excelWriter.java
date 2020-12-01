package net.lingala.zip4j;
import java.io.File;
import java.io.IOException;
import java.sql.ResultSet;
import java.sql.SQLException;
import java.util.ArrayList;

import jxl.Workbook;
import jxl.write.Label;
import jxl.write.WritableSheet;
import jxl.write.WritableWorkbook;
import jxl.write.WriteException;

/**
 * jxl写Excel
 *
 * @author jianggujin
 *
 */
public class excelWriter{
    private String FileName;
    private String FilePath;
    private File xlsFile;
    private WritableWorkbook workbook;
    private WritableSheet sheet;
    private int pointer=0;
    private String sqlPath;

    excelWriter(String fileName,String filePath, String sqlPath) throws IOException, WriteException {
        this.FileName = fileName;
        this.FilePath = filePath;
        this.sqlPath = sqlPath;
        init();
    }

    private void init() throws IOException, WriteException {
        this.xlsFile = new File(this.FilePath +"\\"+this.FileName);
        this.workbook = Workbook.createWorkbook(xlsFile);
        this.sheet = workbook.createSheet("sheet1", 0);
        writeTitle();
    }

    private void writeTitle() throws WriteException, IOException {
        sheet.addCell(new Label(0,0,"Zip_ID"));
        sheet.addCell(new Label(1,0,"Folder_Name"));
        sheet.addCell(new Label(2,0,"PassWord"));
        sheet.addCell(new Label(3,0,"create_Time"));
        pointer++;

    }

    public void AddRow(String ... strings) throws IOException,WriteException{
        int i = 0;
        for (String s : strings){
            sheet.addCell(new Label(i,pointer,s));
            i++;
        }
        pointer++;

    }

    public void close() throws IOException, WriteException {
        workbook.close();
    }

    public void push() throws IOException {
        workbook.write();
    }

    public void printAllData() throws SQLException, IOException, WriteException {
        sqliteConnecter sqliteConnecter = new sqliteConnecter(this.sqlPath+"db.sqlite3");
        ResultSet resultSet = sqliteConnecter.selectAllData();
        writeTitle();
        while (resultSet.next()){
            ArrayList arr = new ArrayList(4);
            arr.add(resultSet.getInt("id"));
            arr.add(resultSet.getString("FolderName"));
            arr.add(resultSet.getString("PassWord"));
            arr.add(resultSet.getString("createTime"));
            AddRow(arr.get(0).toString(),arr.get(1).toString(),arr.get(2).toString(),arr.get(3).toString());
        }
        push();
    }



}